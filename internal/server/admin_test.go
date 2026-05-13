package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminIndexWithoutBearer(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	h := NewHandlers(st)
	srv := httptest.NewServer(routeAdminFirst(h.AdminHandler("admintok"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sync", http.StatusBadRequest)
	})))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestAdminAPIRequiresBearer(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	h := NewHandlers(st)
	srv := httptest.NewServer(routeAdminFirst(h.AdminHandler("secret"), AuthMiddleware("synctok", http.NewServeMux())))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/api/v1/status", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", res.StatusCode)
	}
}

func TestAdminPutProfileValidatesNames(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	h := NewHandlers(st)
	srv := httptest.NewServer(routeAdminFirst(h.AdminHandler("admintok"), http.HandlerFunc(http.NotFound)))
	t.Cleanup(srv.Close)

	body := `{"version":1,"emulators":[{"name":"bad name!","process_names":["x"],"save_paths":["y"]}]}`
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/api/v1/profile", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admintok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", res.StatusCode)
	}
}

func TestAdminPutProfileRoundTrip(t *testing.T) {
	st := NewStorage(t.TempDir(), 10)
	h := NewHandlers(st)
	srv := httptest.NewServer(routeAdminFirst(h.AdminHandler("admintok"), http.HandlerFunc(http.NotFound)))
	t.Cleanup(srv.Close)

	doc := `{"version":1,"emulators":[{"name":"retroarch","process_names":["retroarch"],"save_paths":["retroarch/saves"]}]}`
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/api/v1/profile", strings.NewReader(doc))
	req.Header.Set("Authorization", "Bearer admintok")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put status %d", res.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/api/v1/profile", nil)
	req2.Header.Set("Authorization", "Bearer admintok")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("get status %d", res2.StatusCode)
	}
	var got ProfileDocument
	if err := json.NewDecoder(res2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Emulators) != 1 || got.Emulators[0].Name != "retroarch" {
		t.Fatalf("%+v", got)
	}
}
