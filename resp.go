package resp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/h2non/filetype/types"
	"github.com/thamaji/terrors"
	"gopkg.in/h2non/filetype.v1"
)

func New(w http.ResponseWriter) *Resp {
	return &Resp{w: w, errorHandler: DefaultErrorHandler}
}

type Resp struct {
	w            http.ResponseWriter
	errorHandler ErrorHandler
	cors         bool
}

func (r *Resp) Header() http.Header {
	return r.w.Header()
}

func (r *Resp) SetErrorHandler(handler ErrorHandler) {
	r.errorHandler = handler
}

func (r *Resp) SetCORS(cors bool) {
	r.cors = cors
}

type ErrorHandler func(http.ResponseWriter, error)

var DefaultErrorHandler ErrorHandler = HandleError

func DetectStatusCode(err error) int {
	if os.IsNotExist(err) {
		return http.StatusNotFound
	}

	if os.IsPermission(err) {
		return http.StatusForbidden
	}

	switch terrors.TypeOf(err) {
	case terrors.TypeInternal:
		return http.StatusInternalServerError

	case terrors.TypeInvalid:
		return http.StatusBadRequest

	case terrors.TypeNotExist:
		return http.StatusNotFound

	case terrors.TypePermission:
		return http.StatusForbidden

	case terrors.TypeUnauthorized:
		return http.StatusUnauthorized
	}

	return http.StatusInternalServerError
}

func HandleError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), DetectStatusCode(err))
}

func WriteError(w http.ResponseWriter, err error) {
	New(w).WriteError(err)
}

func (r *Resp) WriteError(err error) {
	if r.cors {
		r.Header().Set("Access-Control-Allow-Origin", "*")
		r.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	}
	if r.errorHandler != nil {
		r.errorHandler(r.w, err)
	} else {
		HandleError(r.w, err)
	}
}

func WriteUnauthorized(w http.ResponseWriter, realm string) {
	New(w).WriteUnauthorized(realm)
}

func (r *Resp) WriteUnauthorized(realm string) {
	if r.cors {
		r.Header().Set("Access-Control-Allow-Origin", "*")
		r.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	}
	r.w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	r.w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(r.w, http.StatusText(http.StatusUnauthorized))
}

func WriteFile(w http.ResponseWriter, statusCode int, path string) {
	New(w).WriteFile(statusCode, path)
}

func (r *Resp) WriteFile(statusCode int, path string) {
	f, err := os.Open(path)
	if err != nil {
		r.WriteError(err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		r.WriteError(err)
		return
	}

	r.w.Header().Set("Last-Modified", fi.ModTime().Format(time.RFC1123))
	r.w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	r.Copy(statusCode, f)
}

func WriteBytes(w http.ResponseWriter, statusCode int, body []byte) {
	New(w).WriteBytes(statusCode, body)
}

func (r *Resp) WriteBytes(statusCode int, body []byte) {
	r.w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	r.Copy(statusCode, bytes.NewReader(body))
}

func WriteJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	New(w).WriteJSON(statusCode, v)
}

func (r *Resp) WriteJSON(statusCode int, v interface{}) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		r.WriteError(err)
		return
	}

	r.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.w.Header().Set("X-Content-Type-Options", "nosniff")
	r.w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	r.Copy(statusCode, bytes.NewReader(body))
}

func WriteText(w http.ResponseWriter, statusCode int, text string) {
	New(w).WriteText(statusCode, text)
}

func (r *Resp) WriteText(statusCode int, text string) {
	body := []byte(text)

	r.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	r.w.Header().Set("X-Content-Type-Options", "nosniff")
	r.w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	r.Copy(statusCode, bytes.NewReader(body))
}

func Copy(w http.ResponseWriter, statusCode int, body io.Reader) {
	New(w).Copy(statusCode, body)
}

func (r *Resp) Copy(statusCode int, body io.Reader) {
	if r.cors {
		r.Header().Set("Access-Control-Allow-Origin", "*")
		r.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	}

	if r.w.Header().Get("Content-Type") == "" {
		reader, contentType, err := DetectContentType(body)
		if err != nil {
			r.WriteError(err)
			return
		}

		body = reader

		r.w.Header().Set("Content-Type", contentType)
	}

	r.w.WriteHeader(statusCode)
	io.Copy(r.w, body)
}

const headerSize = 261

func DetectContentType(r io.Reader) (io.Reader, string, error) {
	var buf [headerSize]byte
	l, _ := io.ReadFull(r, buf[:])
	head := buf[:l]
	r = io.MultiReader(bytes.NewReader(head), r)
	t, err := filetype.Match(head)
	if err != nil || t == types.Unknown {
		return r, "application/octet-stream", err
	}
	return r, t.MIME.Value, nil
}
