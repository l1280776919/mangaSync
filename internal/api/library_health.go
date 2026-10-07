package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) libraryHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.eng.HealthStatus())
}
func (s *Server) startLibraryHealth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   int64 `json:"id"`
		Deep bool  `json:"deep"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.ID < 0 {
		writeErr(w, 400, "体检参数不合法")
		return
	}
	result, err := s.eng.StartHealth(body.ID, body.Deep)
	if err != nil {
		writeErr(w, 409, "%v", err)
		return
	}
	writeJSON(w, 202, result)
}
func (s *Server) cancelLibraryHealth(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err == nil {
		err = s.eng.CancelHealth(id)
	}
	if err != nil {
		writeErr(w, 409, "%v", err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) repairLibraryHealth(w http.ResponseWriter, r *http.Request) {
	id, err := idOf(r, "id")
	if err != nil {
		writeErr(w, 400, "漫画 ID 不合法")
		return
	}
	var body struct {
		RunID int64 `json:"runId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		writeErr(w, 400, "修复参数不合法")
		return
	}
	job, err := s.eng.RepairHealth(body.RunID, id)
	if err != nil {
		writeErr(w, 409, "%v", err)
		return
	}
	writeJSON(w, 202, map[string]int64{"id": job})
}
