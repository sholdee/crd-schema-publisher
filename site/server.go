package site

import (
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

// NewStaticHandler serves generated site files from serveDir.
// The generated _meta directory is internal and intentionally hidden.
func NewStaticHandler(serveDir, basePath string) http.Handler {
	fileHandler := hideMeta(http.FileServer(http.Dir(serveDir)))
	if basePath == "" {
		return fileHandler
	}

	mux := http.NewServeMux()
	mux.Handle(basePath+"/", http.StripPrefix(basePath, fileHandler))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, basePath+"/", http.StatusFound)
	})
	return mux
}

func hideMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "/_meta" || strings.HasPrefix(clean, "/_meta/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *accessLogResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessLogResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}

func (w *accessLogResponseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *accessLogResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func WithAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &accessLogResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		slog.Info("site request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode(),
			"bytes", recorder.bytes,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

func StartServer(addr string, handler http.Handler) (*http.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("site server error", "error", err)
		}
	}()
	return server, nil
}
