package api

import "net/http"

func (s *Server) trapReady() bool {
	return s.Trap != nil && s.Trap.Status().OK
}

func (s *Server) showChecklist(w http.ResponseWriter, r *http.Request) {
	checklist, err := s.Engine.ShowChecklist(r.Context(), s.trapReady(), prefersEnglish(r.Header.Get("Accept-Language")))
	if err != nil {
		writeErr(w, 500, "本番前チェックを取得できません", err.Error(), "時間をおいて再試行してください")
		return
	}
	w.Header().Add("Vary", "Accept-Language")
	writeJSON(w, 200, checklist)
}
