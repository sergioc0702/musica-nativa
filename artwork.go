package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"musica-nativa/internal/library"
)

// artworkPathPrefix is the WebView route for stored pictures: covers and theme
// backgrounds travel as short URLs instead of inline data.
const artworkPathPrefix = "/artwork/"

func artworkURL(id string) string {
	if id == "" {
		return ""
	}
	return artworkPathPrefix + id
}

// artworkFromDataURL decodes a picture chosen in the interface.
func artworkFromDataURL(value string) (library.Artwork, error) {
	header, payload, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return library.Artwork{}, fmt.Errorf("la imagen de fondo no es válida")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return library.Artwork{}, fmt.Errorf("la imagen de fondo no es válida")
	}
	art, ok := library.NewArtwork(data)
	if !ok {
		return library.Artwork{}, fmt.Errorf("la imagen de fondo debe ser JPG, PNG o WebP de hasta 10 MB")
	}
	return art, nil
}

// artworkMiddleware serves GET /artwork/{id} from the store and passes every
// other request to the app's assets.
func artworkMiddleware(store *Store) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := strings.CutPrefix(r.URL.Path, artworkPathPrefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
				return
			}
			art, err := store.Artwork(r.Context(), id)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, "no se pudo leer la imagen", http.StatusInternalServerError)
				return
			}
			header := w.Header()
			header.Set("Content-Type", art.MIME)
			// The id is derived from the bytes, so a URL never changes content.
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			header.Set("X-Content-Type-Options", "nosniff")
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(art.Data))
		})
	}
}
