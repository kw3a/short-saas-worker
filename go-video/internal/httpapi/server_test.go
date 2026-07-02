package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viralshort/go-video/internal/queue"
	"github.com/viralshort/go-video/internal/render"
)

type fakeSubmitter struct {
	accept bool
	count  int
}

func (f *fakeSubmitter) Submit(_ queue.Job) bool {
	if !f.accept {
		return false
	}
	f.count++
	return true
}

const secret = "test-secret"

func newTestServer(sub Submitter) http.Handler {
	return NewServer(secret, &render.Deps{}, sub).Handler()
}

func do(t *testing.T, h http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

const validNarration = `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"hello world","bg_video":"gtav","voice":"en-US-BrianNeural"}`

func TestAuth(t *testing.T) {
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)

	if rr := do(t, h, http.MethodPost, "/generation/narration", "", validNarration); rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth: want 401, got %d", rr.Code)
	}
	if rr := do(t, h, http.MethodPost, "/generation/narration", "Bearer wrong", validNarration); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong auth: want 401, got %d", rr.Code)
	}
	if sub.count != 0 {
		t.Fatalf("nothing should have been enqueued, got %d", sub.count)
	}
}

func TestNarrationValidAndEnqueues(t *testing.T) {
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)

	rr := do(t, h, http.MethodPost, "/generation/narration", "Bearer "+secret, validNarration)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"Ok"`) {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if sub.count != 1 {
		t.Fatalf("want 1 enqueue, got %d", sub.count)
	}
}

func TestUnknownFieldsIgnored(t *testing.T) {
	// Pydantic ignores extra fields; we match that instead of rejecting them.
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)
	body := `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"hi","bg_video":"gtav","voice":"en-US-BrianNeural","x":1}`
	if rr := do(t, h, http.MethodPost, "/generation/narration", "Bearer "+secret, body); rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if sub.count != 1 {
		t.Fatalf("want 1 enqueue, got %d", sub.count)
	}
}

func TestMultibyteLengthCountedAsRunes(t *testing.T) {
	// 1000 'ñ' chars = 2000 bytes but 1000 runes: must be accepted (limit is 2000 chars).
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)
	script := strings.Repeat("ñ", 1000)
	body := `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"` + script + `","bg_video":"gtav","voice":"es-MX-JorgeNeural"}`
	if rr := do(t, h, http.MethodPost, "/generation/narration", "Bearer "+secret, body); rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestValidationErrors(t *testing.T) {
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)
	auth := "Bearer " + secret

	cases := map[string]string{
		"bad voice":      `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"hi","bg_video":"gtav","voice":"nope"}`,
		"bad bg":         `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"hi","bg_video":"nope","voice":"en-US-BrianNeural"}`,
		"bad uuid":       `{"id":"not-a-uuid","script":"hi","bg_video":"gtav","voice":"en-US-BrianNeural"}`,
		"empty script":   `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","script":"","bg_video":"gtav","voice":"en-US-BrianNeural"}`,
		"malformed json": `{not json`,
	}
	for name, body := range cases {
		rr := do(t, h, http.MethodPost, "/generation/narration", auth, body)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d (%s)", name, rr.Code, rr.Body.String())
		}
	}
	if sub.count != 0 {
		t.Fatalf("invalid requests should not enqueue, got %d", sub.count)
	}
}

func TestQueueFull(t *testing.T) {
	sub := &fakeSubmitter{accept: false}
	h := newTestServer(sub)
	rr := do(t, h, http.MethodPost, "/generation/narration", "Bearer "+secret, validNarration)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestAskRedditValidation(t *testing.T) {
	sub := &fakeSubmitter{accept: true}
	h := newTestServer(sub)
	auth := "Bearer " + secret

	valid := `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","title":"Best advice?","comments":["a","b"],"bg_video":"minecraft","voice":"es-MX-JorgeNeural"}`
	if rr := do(t, h, http.MethodPost, "/generation/askreddit", auth, valid); rr.Code != http.StatusOK {
		t.Fatalf("valid askreddit: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}

	noComments := `{"id":"8f7a302f-ec99-457b-8fe2-20476f50fe4b","title":"t","comments":[],"bg_video":"minecraft","voice":"es-MX-JorgeNeural"}`
	if rr := do(t, h, http.MethodPost, "/generation/askreddit", auth, noComments); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty comments: want 400, got %d", rr.Code)
	}
}
