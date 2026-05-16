package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		token      string
		authHeader string
		wantStatus int
	}{
		{
			name:       "no_token_configured_passes",
			token:      "",
			authHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_token",
			token:      "secret123",
			authHeader: "Bearer secret123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_token_lowercase_bearer",
			token:      "secret123",
			authHeader: "bearer secret123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid_token_quoted_credential",
			token:      "secret123",
			authHeader: `Bearer "secret123"`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong_token",
			token:      "secret123",
			authHeader: "Bearer wrongtoken",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing_header",
			token:      "secret123",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "not_bearer_scheme",
			token:      "secret123",
			authHeader: "Basic xyz",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty_bearer_value",
			token:      "secret123",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := AuthMiddleware(tt.token, okHandler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}
