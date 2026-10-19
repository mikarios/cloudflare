package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFindMyIP(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantIP     string
		wantErrSub string // substring expected in the error; "" means no error
	}{
		{
			name: "happy path returns IPv4",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("203.0.113.42"))
			},
			wantIP: "203.0.113.42",
		},
		{
			name: "trims surrounding whitespace from body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("  203.0.113.7\n"))
			},
			wantIP: "203.0.113.7",
		},
		{
			name: "returns IPv6 when response is IPv6",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("2001:db8::1"))
			},
			wantIP: "2001:db8::1",
		},
		{
			name: "rejects garbage body even on 200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("rate limited"))
			},
			wantErrSub: "could not parse ip",
		},
		{
			name: "rejects empty body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantErrSub: "could not parse ip",
		},
		{
			name: "returns error on 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("upstream down"))
			},
			wantErrSub: "unexpected status 500",
		},
		{
			name: "returns error on 429",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErrSub: "unexpected status 429",
		},
		{
			name: "returns error on 404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErrSub: "unexpected status 404",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := &http.Client{Timeout: 2 * time.Second}
			gotIP, err := findMyIP(client, srv.URL)

			if tc.wantErrSub == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if gotIP != tc.wantIP {
					t.Errorf("got IP %q, want %q", gotIP, tc.wantIP)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil (IP=%q)", tc.wantErrSub, gotIP)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErrSub)
			}
			if gotIP != "" {
				t.Errorf("expected empty IP on error, got %q", gotIP)
			}
		})
	}
}

func TestFindMyIP_ClientDoError(t *testing.T) {
	// Point at a URL that will fail at the transport layer (no listener).
	client := &http.Client{Timeout: 2 * time.Second}
	_, err := findMyIP(client, "http://127.0.0.1:1/never-listening")
	if err == nil {
		t.Fatal("expected error from failed Do(), got nil")
	}
	if !strings.Contains(err.Error(), "could not get site") {
		t.Errorf("error %q does not contain expected substring", err.Error())
	}
}

func TestFindMyIP_Timeout(t *testing.T) {
	// Server that blocks longer than the client timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	start := time.Now()
	_, err := findMyIP(client, srv.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("findMyIP did not return promptly on timeout: took %v", elapsed)
	}
}
