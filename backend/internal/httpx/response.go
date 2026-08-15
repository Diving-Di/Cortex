package httpx

import (
    "encoding/json"
    "errors"
    "io"
    "log/slog"
    "net/http"

    "cortex/backend/internal/apierror"
)

func JSON(w http.ResponseWriter, status int, value any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    if status != http.StatusNoContent {
        _ = json.NewEncoder(w).Encode(value)
    }
}

func DecodeJSON(r *http.Request, target any) *apierror.Error {
    decoder := json.NewDecoder(io.LimitReader(r.Body, (1<<20)+1))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(target); err != nil {
        return apierror.Validation([]map[string]any{{"msg": err.Error()}})
    }
    if decoder.Decode(&struct{}{}) == nil {
        return apierror.Validation([]map[string]any{{"msg": "request body must contain one JSON object"}})
    }
    return nil
}

func WriteError(w http.ResponseWriter, logger *slog.Logger, err error) {
    var appErr *apierror.Error
    if errors.As(err, &appErr) {
        JSON(w, appErr.StatusCode, map[string]any{
            "code": appErr.Code, "message": appErr.Message, "details": appErr.Details,
        })
        return
    }
    logger.Error("request failed", "error", err)
    JSON(w, http.StatusInternalServerError, map[string]any{
        "code": "INTERNAL_ERROR", "message": "服务器内部错误", "details": nil,
    })
}
