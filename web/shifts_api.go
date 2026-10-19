package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/go-chi/chi/v5"
)

func (w *Web) requireAdmin(rw http.ResponseWriter, r *http.Request) bool {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Login required")
		return false
	}
	if !u.IsAdmin {
		writeError(rw, http.StatusForbidden, "Admin role ('g::t::orgs') required")
		return false
	}
	return true
}

func (w *Web) handleGetShiftDefinitions(rw http.ResponseWriter, r *http.Request) {
	defs, err := w.storage.GetShiftDefinitions()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, defs)
}

func (w *Web) handlePostShiftDefinition(rw http.ResponseWriter, r *http.Request) {
	if !w.requireAdmin(rw, r) {
		return
	}

	var req storage.ShiftDefinition
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(rw, http.StatusBadRequest, "Shift name is required")
		return
	}

	created, err := w.storage.CreateShiftDefinition(req)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusCreated, created)
}

func (w *Web) handlePutShiftDefinition(rw http.ResponseWriter, r *http.Request) {
	if !w.requireAdmin(rw, r) {
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	var req storage.ShiftDefinition
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	req.ID = uint(id)

	if err := w.storage.UpdateShiftDefinition(req); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) handleDeleteShiftDefinition(rw http.ResponseWriter, r *http.Request) {
	if !w.requireAdmin(rw, r) {
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := w.storage.DeleteShiftDefinition(uint(id)); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func (w *Web) handleGetShifts(rw http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	upcoming := r.URL.Query().Get("upcoming")

	var shiftsList []storage.Shift
	var err error

	if upcoming == "true" {
		shiftsList, err = w.storage.GetUpcomingShifts(time.Now().Add(-2 * time.Hour))
	} else {
		shiftsList, err = w.storage.GetShifts(dateStr)
	}

	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, shiftsList)
}

func (w *Web) handlePostGenerateShifts(rw http.ResponseWriter, r *http.Request) {
	if !w.requireAdmin(rw, r) {
		return
	}

	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	type GenReq struct {
		Date   string `json:"date"`
		Freeze bool   `json:"freeze"`
	}
	var req GenReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	targetDate := time.Now().AddDate(0, 0, 1) // default tomorrow
	if req.Date != "" {
		parsed, err := time.Parse("2006-01-02", req.Date)
		if err == nil {
			targetDate = parsed
		}
	}

	shiftsList, err := w.shifts.GenerateShiftsForDate(targetDate, req.Freeze)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, "Generation failed: "+err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]any{
		"message": "Schedule generated successfully",
		"date":    targetDate.Format("2006-01-02"),
		"count":   len(shiftsList),
		"shifts":  shiftsList,
	})
}

func (w *Web) handlePostFreezeShifts(rw http.ResponseWriter, r *http.Request) {
	if !w.requireAdmin(rw, r) {
		return
	}

	type FreezeReq struct {
		Date string `json:"date"`
	}
	var req FreezeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Date == "" {
		writeError(rw, http.StatusBadRequest, "Date (YYYY-MM-DD) is required")
		return
	}

	if err := w.storage.FreezeShiftsForDate(req.Date); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{
		"message": "Schedule frozen successfully for " + req.Date,
	})
}

func (w *Web) handlePostShiftCantAttend(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Login required")
		return
	}

	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	type DropReq struct {
		AssignmentID uint   `json:"assignment_id"`
		Reason       string `json:"reason"`
	}
	var req DropReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AssignmentID == 0 {
		writeError(rw, http.StatusBadRequest, "assignment_id is required")
		return
	}

	// Verify authorization: must be user's assignment or admin
	assignment, err := w.storage.GetShiftAssignment(req.AssignmentID)
	if err != nil || assignment == nil {
		writeError(rw, http.StatusNotFound, "Assignment not found")
		return
	}
	if assignment.UserId != u.DiscordId && !u.IsAdmin {
		writeError(rw, http.StatusForbidden, "Cannot modify another user's shift")
		return
	}

	reason := req.Reason
	if reason == "" {
		reason = "Unavailable"
	}

	replacement, err := w.shifts.CanAttendShift(req.AssignmentID, reason)
	if err != nil {
		writeJSON(rw, http.StatusOK, map[string]any{
			"status":  "dropped_unreplaced",
			"message": "Shift assignment dropped, but no immediate replacement found: " + err.Error(),
		})
		return
	}

	writeJSON(rw, http.StatusOK, map[string]any{
		"status":      "reassigned",
		"message":     "Shift successfully reassigned to replacement attendee",
		"replacement": replacement,
	})
}

func (w *Web) handlePostShiftSwap(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Login required")
		return
	}

	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	type SwapReq struct {
		AssignmentID uint   `json:"assignment_id"`
		TargetUserID string `json:"target_user_id"`
	}
	var req SwapReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AssignmentID == 0 || req.TargetUserID == "" {
		writeError(rw, http.StatusBadRequest, "assignment_id and target_user_id are required")
		return
	}

	assignment, err := w.storage.GetShiftAssignment(req.AssignmentID)
	if err != nil || assignment == nil {
		writeError(rw, http.StatusNotFound, "Assignment not found")
		return
	}
	if assignment.UserId != u.DiscordId && !u.IsAdmin {
		writeError(rw, http.StatusForbidden, "Cannot swap another user's shift")
		return
	}

	swapRequest, err := w.shifts.RequestSwap(req.AssignmentID, req.TargetUserID)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, swapRequest)
}

func (w *Web) handlePostShiftAcceptSwap(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Login required")
		return
	}

	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	swapIDStr := chi.URLParam(r, "swap_id")
	swapID, err := strconv.Atoi(swapIDStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid swap_id")
		return
	}

	if err := w.shifts.AcceptSwap(uint(swapID), u.DiscordId); err != nil {
		writeError(rw, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{
		"message": "Shift swap successfully completed",
	})
}

func (w *Web) handlePostCompleteShift(rw http.ResponseWriter, r *http.Request) {
	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid shift ID")
		return
	}

	if err := w.shifts.CompleteShift(uint(id)); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, map[string]string{
		"message": "Shift marked as completed and standby minutes credited",
	})
}

func (w *Web) handleGetFairnessStats(rw http.ResponseWriter, r *http.Request) {
	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	overview, err := w.shifts.GetFairnessOverview()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, overview)
}

func (w *Web) handleGetRegistrations(rw http.ResponseWriter, r *http.Request) {
	regs, err := w.storage.GetUserRegistrations("")
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, regs)
}

func (w *Web) handleGetCalendarSchedule(rw http.ResponseWriter, r *http.Request) {
	if w.shifts == nil {
		writeError(rw, http.StatusServiceUnavailable, "Shifts service not available")
		return
	}

	eventCode := r.URL.Query().Get("event")
	pred, err := w.shifts.PredictEventSchedule(eventCode)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, pred)
}

