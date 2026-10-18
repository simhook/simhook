package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapErrDataException(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"nul byte in text", fmt.Errorf("tx: %w", &pgconn.PgError{Code: "22021", Message: `invalid byte sequence for encoding "UTF8": 0x00`}), http.StatusUnprocessableEntity, "validation_failed"},
		{"bad unicode escape in jsonb", &pgconn.PgError{Code: "22P05"}, http.StatusUnprocessableEntity, "validation_failed"},
		{"other database error stays hidden", &pgconn.PgError{Code: "23503"}, http.StatusInternalServerError, "internal_error"},
		{"plain error stays hidden", errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiE *APIError
			if err := mapErr(context.Background(), log, tc.err); !errors.As(err, &apiE) {
				t.Fatalf("not an APIError: %v", err)
			}
			if apiE.Status != tc.want || apiE.Code != tc.code {
				t.Fatalf("got %d %s, want %d %s", apiE.Status, apiE.Code, tc.want, tc.code)
			}
		})
	}
}
