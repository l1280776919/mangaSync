package api

import (
	"encoding/json"
	"github.com/l1280776919/mangaSync/internal/store"
	"net/http"
	"time"
)

func (s *Server) readingHistory(w http.ResponseWriter, r *http.Request) {
	items, e := s.st.Reading(sessionFrom(r).UserID, "", "")
	if e != nil {
		writeErr(w, 500, "%v", e)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) getReading(w http.ResponseWriter, r *http.Request) {
	items, e := s.st.Reading(sessionFrom(r).UserID, r.PathValue("kind"), r.PathValue("comicId"))
	if e != nil {
		writeErr(w, 500, "%v", e)
		return
	}
	if len(items) == 0 {
		writeJSON(w, 200, nil)
		return
	}
	writeJSON(w, 200, items[0])
}
func (s *Server) putReading(w http.ResponseWriter, r *http.Request) {
	var p store.ReadingProgress
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&p); e != nil {
		writeErr(w, 400, "阅读进度格式错误")
		return
	}
	p.Kind = r.PathValue("kind")
	p.ComicID = r.PathValue("comicId")
	if (p.Kind != "pica" && p.Kind != "jm") || p.Order < 1 || p.Order > 100000 || p.Page < 1 || p.Page > 5000 || len(p.Title) > 2048 || len(p.ComicID) > 512 || p.UpdatedAt <= 0 || p.UpdatedAt > time.Now().Add(5*time.Minute).UnixMilli() {
		writeErr(w, 400, "阅读进度参数不合法")
		return
	}
	if e := s.st.SaveReading(sessionFrom(r).UserID, p); e != nil {
		writeErr(w, 500, "%v", e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) syncHistory(w http.ResponseWriter, r *http.Request) {
	x, e := s.st.SyncHistory()
	if e != nil {
		writeErr(w, 500, "%v", e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) trashList(w http.ResponseWriter, r *http.Request) {
	x, e := s.st.TrashItems()
	if e != nil {
		writeErr(w, 500, "%v", e)
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) restoreTrash(w http.ResponseWriter, r *http.Request) {
	id, e := idOf(r, "id")
	if e == nil {
		e = s.eng.RestoreTrash(id)
	}
	if e != nil {
		writeErr(w, 409, "%v", e)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
