package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A JSON request against the dev-mode server, which signs every request in
// as the same person.
func jsonReq(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	var out map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

func mustCreateLink(t *testing.T, h http.Handler, body string) map[string]interface{} {
	t.Helper()
	rr, out := jsonReq(t, h, http.MethodPost, "/api/links", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create link: %d %s", rr.Code, rr.Body.String())
	}
	return out["link"].(map[string]interface{})
}

func mustCreateFolder(t *testing.T, h http.Handler, name string) string {
	t.Helper()
	rr, out := jsonReq(t, h, http.MethodPost, "/api/folders", `{"name":"`+name+`"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create folder: %d %s", rr.Code, rr.Body.String())
	}
	return out["id"].(string)
}

// Editing a link changes everything but its address: the destination is
// cleaned again, the folder moves, zero clears a budget or an expiry, and an
// empty PIN removes it.
func TestEditLinkEverythingButTheAddress(t *testing.T) {
	h, _, _, done := setupTestServer(t)
	defer done()
	work := mustCreateFolder(t, h, "Work")
	link := mustCreateLink(t, h, `{"url":"https://example.com/a?utm_source=x&id=1","custom_slug":"edit-me","pin":"1234","max_clicks":10,"expires_in_hours":24}`)
	id := link["id"].(string)

	rr, out := jsonReq(t, h, http.MethodPatch, "/api/links/"+id,
		`{"target_url":"https://example.com/b?fbclid=zzz&page=2","title":"Renamed","folder_id":"`+work+`","tags":["x","y"],"redirect_type":301,"max_clicks":0,"expires_at":0,"pin":"","ios_url":"https://apps.apple.com/app/id1?utm_campaign=c"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rr.Code, rr.Body.String())
	}
	if out["slug"] != "edit-me" {
		t.Errorf("slug changed to %v", out["slug"])
	}
	if out["target_url"] != "https://example.com/b?page=2" {
		t.Errorf("destination not cleaned: %v", out["target_url"])
	}
	if out["folder_id"] != work || out["title"] != "Renamed" || out["redirect_type"] != float64(301) {
		t.Errorf("fields not applied: %v", out)
	}
	if _, has := out["max_clicks"]; has {
		t.Errorf("click budget should be cleared by zero, got %v", out["max_clicks"])
	}
	if _, has := out["expires_at"]; has {
		t.Errorf("expiry should be cleared by zero, got %v", out["expires_at"])
	}
	if out["has_pin"] != false {
		t.Errorf("empty pin should remove the PIN")
	}
	if out["ios_url"] != "https://apps.apple.com/app/id1" {
		t.Errorf("iOS target not cleaned: %v", out["ios_url"])
	}

	// Moving out of the folder with "" and re-arming a PIN.
	rr, out = jsonReq(t, h, http.MethodPatch, "/api/links/"+id, `{"folder_id":"","pin":"9876","expires_in_hours":2}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("second patch: %d %s", rr.Code, rr.Body.String())
	}
	if _, has := out["folder_id"]; has {
		t.Errorf("empty folder_id should leave the folder, got %v", out["folder_id"])
	}
	if out["has_pin"] != true || out["expires_at"] == nil {
		t.Errorf("PIN and expiry not re-armed: %v", out)
	}

	// The redirect type is validated, and a folder that does not exist is refused.
	if rr, _ := jsonReq(t, h, http.MethodPatch, "/api/links/"+id, `{"redirect_type":307}`); rr.Code != http.StatusBadRequest {
		t.Errorf("307 accepted: %d", rr.Code)
	}
	if rr, _ := jsonReq(t, h, http.MethodPatch, "/api/links/"+id, `{"folder_id":"no-such-folder"}`); rr.Code != http.StatusBadRequest {
		t.Errorf("unknown folder accepted: %d", rr.Code)
	}
}

// Deleting a folder keeps its links and sends them back to no folder;
// renaming keeps everything in place.
func TestFolderDeleteKeepsLinksAndRenameWorks(t *testing.T) {
	h, _, _, done := setupTestServer(t)
	defer done()
	old := mustCreateFolder(t, h, "Old")
	link := mustCreateLink(t, h, `{"url":"https://example.com/in-folder","folder_id":"`+old+`"}`)
	id := link["id"].(string)
	if link["folder_id"] != old {
		t.Fatalf("link not in folder: %v", link["folder_id"])
	}

	if rr, out := jsonReq(t, h, http.MethodPatch, "/api/folders/"+old, `{"name":"Older","color":"#22c55e"}`); rr.Code != http.StatusOK || out["name"] != "Older" || out["color"] != "#22c55e" {
		t.Fatalf("rename: %d %s", rr.Code, rr.Body.String())
	}
	if rr, _ := jsonReq(t, h, http.MethodPatch, "/api/folders/"+old, `{"name":"   "}`); rr.Code != http.StatusBadRequest {
		t.Errorf("blank name accepted: %d", rr.Code)
	}

	if rr, _ := jsonReq(t, h, http.MethodDelete, "/api/folders/"+old, ""); rr.Code != http.StatusOK {
		t.Fatalf("delete folder: %d %s", rr.Code, rr.Body.String())
	}
	rr, out := jsonReq(t, h, http.MethodGet, "/api/links/"+id, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("link gone with its folder: %d", rr.Code)
	}
	if _, has := out["folder_id"]; has {
		t.Errorf("link still points at the deleted folder: %v", out["folder_id"])
	}
	if rr, _ := jsonReq(t, h, http.MethodDelete, "/api/folders/"+old, ""); rr.Code != http.StatusBadRequest {
		t.Errorf("deleting twice should fail: %d", rr.Code)
	}
}

// The dashboard offers the controls: an Edit button per link, and rename and
// delete for the folder that is selected — and only then.
func TestDashboardShowsManagementControls(t *testing.T) {
	h, _, _, done := setupTestServer(t)
	defer done()
	f := mustCreateFolder(t, h, "Shown")
	mustCreateLink(t, h, `{"url":"https://example.com/x","folder_id":"`+f+`"}`)

	all := get(t, h, "/").Body.String()
	if !strings.Contains(all, `class="btn btn-secondary btn-sm edit-link-btn"`) || !strings.Contains(all, `id="edit-modal"`) {
		t.Errorf("no edit control on the dashboard")
	}
	if strings.Contains(all, `id="delete-folder-btn"`) {
		t.Errorf("folder controls shown with no folder selected")
	}
	one := get(t, h, "/?folder="+f).Body.String()
	if !strings.Contains(one, `id="delete-folder-btn"`) || !strings.Contains(one, `id="rename-folder-btn"`) {
		t.Errorf("folder controls missing with a folder selected")
	}
	if strings.Contains(one, `style="`) {
		t.Errorf("inline style crept back in")
	}
}
