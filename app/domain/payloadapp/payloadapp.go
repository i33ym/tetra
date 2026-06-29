// Package payloadapp maintains the app layer api for the payload domain.
package payloadapp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/app/sdk/query"
	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/sdk/order"
	"github.com/i33ym/tetra/business/sdk/page"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

// textPartLimit caps the size of an inline text form field.
const textPartLimit = 1 << 20 // 1 MiB

// maxJSONBytes caps the size of a JSON request body.
const maxJSONBytes = 1 << 20 // 1 MiB

type app struct {
	log         *logger.Logger
	payloadBus  *payloadbus.Business
	jobBus      *jobbus.Business
	blob        *blob.Store
	db          sqldb.Beginner
	maxUpload   int64
	maxAttempts int
}

func newApp(cfg Config) *app {
	return &app{
		log:         cfg.Log,
		payloadBus:  cfg.PayloadBus,
		jobBus:      cfg.JobBus,
		blob:        cfg.Blob,
		db:          cfg.DB,
		maxUpload:   cfg.MaxUploadBytes,
		maxAttempts: cfg.MaxAttempts,
	}
}

// create ingests a payload (multipart file[+text] or JSON text), stores it, and
// enqueues a processing job. It returns 202 Accepted immediately.
func (a *app) create(ctx context.Context, r *http.Request) web.Encoder {
	contentType := r.Header.Get("Content-Type")

	var np payloadbus.NewPayload
	var err error

	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		np, err = a.parseMultipart(ctx, r)

	case strings.HasPrefix(contentType, "application/json"):
		np, err = parseJSON(r)

	default:
		return errs.Errorf(errs.InvalidArgument, "unsupported content type %q; use multipart/form-data or application/json", contentType)
	}

	if err != nil {
		var appErr *errs.Error
		if errors.As(err, &appErr) {
			return appErr
		}
		return errs.New(errs.InvalidArgument, err)
	}

	// Persist the metadata and enqueue a job atomically. The blob bytes have
	// already been written; an orphaned object on commit failure is acceptable
	// (swept by minio-gc).
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return errs.Errorf(errs.Internal, "begin tx: %s", err)
	}
	defer tx.Rollback(context.Background())

	payloadBus, err := a.payloadBus.NewWithTx(tx)
	if err != nil {
		return errs.Errorf(errs.Internal, "payloadbus tx: %s", err)
	}

	jobBus, err := a.jobBus.NewWithTx(tx)
	if err != nil {
		return errs.Errorf(errs.Internal, "jobbus tx: %s", err)
	}

	p, err := payloadBus.Create(ctx, np)
	if err != nil {
		return errs.Errorf(errs.Internal, "create: %s", err)
	}

	if _, err := jobBus.Enqueue(ctx, jobbus.NewJob{PayloadID: p.ID, MaxAttempts: a.maxAttempts}); err != nil {
		return errs.Errorf(errs.Internal, "enqueue: %s", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return errs.Errorf(errs.Internal, "commit: %s", err)
	}

	return acceptedResponse{ID: p.ID.String(), Status: p.Status.String()}
}

// queryByID returns a single payload and its current status/result.
func (a *app) queryByID(ctx context.Context, r *http.Request) web.Encoder {
	id, err := uuid.Parse(web.Param(r, "payload_id"))
	if err != nil {
		return errs.Errorf(errs.InvalidArgument, "invalid payload id: %q", web.Param(r, "payload_id"))
	}

	p, err := a.payloadBus.QueryByID(ctx, id)
	if err != nil {
		if errors.Is(err, payloadbus.ErrNotFound) {
			return errs.New(errs.NotFound, err)
		}
		return errs.Errorf(errs.Internal, "querybyid: payloadID[%s]: %s", id, err)
	}

	return toAppPayload(p)
}

// query returns a paged list of payloads.
func (a *app) query(ctx context.Context, r *http.Request) web.Encoder {
	qp := parseQueryParams(r)

	pg, err := page.Parse(qp.page, qp.rows)
	if err != nil {
		return errs.NewFieldErrors("page", err)
	}

	filter, err := parseFilter(qp)
	if err != nil {
		return err.(*errs.Error)
	}

	orderBy, err := order.Parse(orderByFields, qp.orderBy, payloadbus.DefaultOrderBy)
	if err != nil {
		return errs.NewFieldErrors("orderBy", err)
	}

	ps, err := a.payloadBus.Query(ctx, filter, orderBy, pg)
	if err != nil {
		return errs.Errorf(errs.Internal, "query: %s", err)
	}

	total, err := a.payloadBus.Count(ctx, filter)
	if err != nil {
		return errs.Errorf(errs.Internal, "count: %s", err)
	}

	return query.NewResult(toAppPayloads(ps), total, pg)
}

// content streams the original uploaded file back to the client.
func (a *app) content(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := uuid.Parse(web.Param(r, "payload_id"))
	if err != nil {
		http.Error(w, "invalid payload id", http.StatusBadRequest)
		return
	}

	p, err := a.payloadBus.QueryByID(ctx, id)
	if err != nil {
		http.Error(w, "payload not found", http.StatusNotFound)
		return
	}

	if p.Kind != payloadbus.KindFile || p.ObjectKey == "" {
		http.Error(w, "payload has no file content", http.StatusNotFound)
		return
	}

	obj, err := a.blob.Get(ctx, p.ObjectKey)
	if err != nil {
		a.log.Error(ctx, "content", "msg", "fetching object", "key", p.ObjectKey, "err", err)
		http.Error(w, "content unavailable", http.StatusInternalServerError)
		return
	}
	defer obj.Body.Close()

	if p.ContentType != "" {
		w.Header().Set("Content-Type", p.ContentType)
	}
	if p.OriginalFilename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", p.OriginalFilename))
	}
	if obj.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	}

	if _, err := io.Copy(w, obj.Body); err != nil {
		a.log.Error(ctx, "content", "msg", "streaming object", "key", p.ObjectKey, "err", err)
	}
}

// =============================================================================

func parseJSON(r *http.Request) (payloadbus.NewPayload, error) {
	var in NewPayloadText
	if err := web.DecodeJSON(r, maxJSONBytes, &in); err != nil {
		return payloadbus.NewPayload{}, err
	}

	return payloadbus.NewPayload{
		Kind:     payloadbus.KindText,
		BodyText: in.Text,
	}, nil
}

// parseMultipart streams the request without buffering the whole file: the text
// field is read with a small cap and the file is streamed straight to object
// storage with a size cap enforced mid-stream.
func (a *app) parseMultipart(ctx context.Context, r *http.Request) (payloadbus.NewPayload, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return payloadbus.NewPayload{}, fmt.Errorf("multipart reader: %w", err)
	}

	np := payloadbus.NewPayload{Kind: payloadbus.KindText}

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return payloadbus.NewPayload{}, fmt.Errorf("next part: %w", err)
		}

		switch part.FormName() {
		case "text":
			b, err := io.ReadAll(io.LimitReader(part, textPartLimit))
			part.Close()
			if err != nil {
				return payloadbus.NewPayload{}, fmt.Errorf("read text part: %w", err)
			}
			np.BodyText = string(b)

		case "file":
			filename := part.FileName()
			if filename == "" {
				part.Close()
				continue
			}

			// Cap the stream at maxUpload+1 so we can detect (and reject)
			// oversize uploads without buffering the whole file.
			br := bufio.NewReader(io.LimitReader(part, a.maxUpload+1))

			head, _ := br.Peek(512)
			ct := part.Header.Get("Content-Type")
			if ct == "" || ct == "application/octet-stream" {
				ct = http.DetectContentType(head)
			}

			objectKey := fmt.Sprintf("payloads/%s/%s", uuid.NewString(), sanitizeFilename(filename))

			size, err := a.blob.Put(ctx, objectKey, br, -1, ct)
			part.Close()
			if err != nil {
				return payloadbus.NewPayload{}, fmt.Errorf("store object: %w", err)
			}

			if size > a.maxUpload {
				_ = a.blob.Remove(ctx, objectKey)
				return payloadbus.NewPayload{}, errs.Errorf(errs.InvalidArgument, "file exceeds max upload size of %d bytes", a.maxUpload)
			}

			np.Kind = payloadbus.KindFile
			np.OriginalFilename = filename
			np.ContentType = ct
			np.ObjectKey = objectKey
			np.SizeBytes = size

		default:
			part.Close()
		}
	}

	if np.Kind == payloadbus.KindText && np.BodyText == "" {
		return payloadbus.NewPayload{}, errs.Errorf(errs.InvalidArgument, "payload must contain a 'text' field or a 'file'")
	}

	return np, nil
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}
