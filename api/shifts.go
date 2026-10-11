package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gdg-garage/garage-trip-chores/shifts"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

type ShiftDefinitionsResponse struct {
	Body []storage.ShiftDefinition
}

type ShiftDefinitionRequest struct {
	Body storage.ShiftDefinition
}

type ShiftsListRequest struct {
	Date     string `query:"date" doc:"Filter by date YYYY-MM-DD"`
	Upcoming bool   `query:"upcoming" doc:"Only upcoming shifts"`
}

type ShiftsListResponse struct {
	Body []storage.Shift
}

type GenerateShiftsRequest struct {
	Body struct {
		Date   string `json:"date" doc:"Target date YYYY-MM-DD"`
		Freeze bool   `json:"freeze" doc:"Whether to freeze after generation"`
	}
}

type GenerateShiftsResponse struct {
	Body struct {
		Message string          `json:"message"`
		Date    string          `json:"date"`
		Count   int             `json:"count"`
		Shifts  []storage.Shift `json:"shifts"`
	}
}

type FreezeShiftsRequest struct {
	Body struct {
		Date string `json:"date" doc:"Date to freeze YYYY-MM-DD"`
	}
}

type FreezeShiftsResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

type CantAttendRequest struct {
	Body struct {
		AssignmentID uint   `json:"assignment_id" doc:"Shift assignment ID"`
		Reason       string `json:"reason" doc:"Reason for not attending"`
	}
}

type CantAttendResponse struct {
	Body struct {
		Status      string                   `json:"status"`
		Message     string                   `json:"message"`
		Replacement *storage.ShiftAssignment `json:"replacement,omitempty"`
	}
}

type ShiftSwapRequestPayload struct {
	Body struct {
		AssignmentID uint   `json:"assignment_id"`
		TargetUserID string `json:"target_user_id"`
	}
}

type ShiftSwapResponse struct {
	Body storage.ShiftSwapRequest
}

type AcceptSwapRequest struct {
	SwapID       uint   `path:"swap_id"`
	TargetUserID string `query:"user_id"`
}

type SimpleMessageResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

type CompleteShiftRequest struct {
	ID uint `path:"id"`
}

type FairnessResponse struct {
	Body map[string]shifts.UserFairnessMetric
}

func (a *Api) registerShiftOperations(api huma.API, shiftsService *shifts.ShiftsService) {
	if shiftsService == nil {
		return
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-shift-definitions",
		Method:      http.MethodGet,
		Path:        "/shifts/definitions",
		Summary:     "Get all shift definitions",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *struct{}) (*ShiftDefinitionsResponse, error) {
		defs, err := a.storage.GetShiftDefinitions()
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &ShiftDefinitionsResponse{Body: defs}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-shift-definition",
		Method:      http.MethodPost,
		Path:        "/shifts/definitions",
		Summary:     "Create a shift definition",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *ShiftDefinitionRequest) (*ShiftDefinitionRequest, error) {
		created, err := a.storage.CreateShiftDefinition(input.Body)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &ShiftDefinitionRequest{Body: created}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-shifts",
		Method:      http.MethodGet,
		Path:        "/shifts",
		Summary:     "List shifts",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *ShiftsListRequest) (*ShiftsListResponse, error) {
		var list []storage.Shift
		var err error
		if input.Upcoming {
			list, err = a.storage.GetUpcomingShifts(time.Now().Add(-2 * time.Hour))
		} else {
			list, err = a.storage.GetShifts(input.Date)
		}
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &ShiftsListResponse{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "generate-shifts",
		Method:      http.MethodPost,
		Path:        "/shifts/generate",
		Summary:     "Generate and balance shifts for a date",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *GenerateShiftsRequest) (*GenerateShiftsResponse, error) {
		targetDate := time.Now().AddDate(0, 0, 1)
		if input.Body.Date != "" {
			if parsed, err := time.Parse("2006-01-02", input.Body.Date); err == nil {
				targetDate = parsed
			}
		}

		shs, err := shiftsService.GenerateShiftsForDate(targetDate, input.Body.Freeze)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}

		resp := &GenerateShiftsResponse{}
		resp.Body.Message = "Schedule generated successfully"
		resp.Body.Date = targetDate.Format("2006-01-02")
		resp.Body.Count = len(shs)
		resp.Body.Shifts = shs
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "freeze-shifts",
		Method:      http.MethodPost,
		Path:        "/shifts/freeze",
		Summary:     "Freeze schedule for a date",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *FreezeShiftsRequest) (*FreezeShiftsResponse, error) {
		if input.Body.Date == "" {
			return nil, huma.Error400BadRequest("Date is required")
		}
		if err := a.storage.FreezeShiftsForDate(input.Body.Date); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		resp := &FreezeShiftsResponse{}
		resp.Body.Message = "Schedule frozen for " + input.Body.Date
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cant-attend-shift",
		Method:      http.MethodPost,
		Path:        "/shifts/cant-attend",
		Summary:     "Declare unable to attend shift; auto-reassigns",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *CantAttendRequest) (*CantAttendResponse, error) {
		replacement, err := shiftsService.CanAttendShift(input.Body.AssignmentID, input.Body.Reason)
		resp := &CantAttendResponse{}
		if err != nil {
			resp.Body.Status = "dropped_unreplaced"
			resp.Body.Message = err.Error()
			return resp, nil
		}
		resp.Body.Status = "reassigned"
		resp.Body.Message = "Replacement attendee assigned"
		resp.Body.Replacement = replacement
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "swap-shift",
		Method:      http.MethodPost,
		Path:        "/shifts/swap",
		Summary:     "Request shift swap with attendee",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *ShiftSwapRequestPayload) (*ShiftSwapResponse, error) {
		req, err := shiftsService.RequestSwap(input.Body.AssignmentID, input.Body.TargetUserID)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &ShiftSwapResponse{Body: *req}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-shift",
		Method:      http.MethodPost,
		Path:        "/shifts/{id}/complete",
		Summary:     "Mark shift as complete",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *CompleteShiftRequest) (*SimpleMessageResponse, error) {
		if err := shiftsService.CompleteShift(input.ID); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		resp := &SimpleMessageResponse{}
		resp.Body.Message = "Shift completed and standby minutes credited"
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-shifts-fairness",
		Method:      http.MethodGet,
		Path:        "/shifts/fairness",
		Summary:     "Get fairness metrics table",
		Tags:        []string{"Shifts"},
	}, func(ctx context.Context, input *struct{}) (*FairnessResponse, error) {
		overview, err := shiftsService.GetFairnessOverview()
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &FairnessResponse{Body: overview}, nil
	})
}
