package raft_handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/wasilak/raft-sample/utils"
)

func (h handler) PassLeadership(eCtx echo.Context) error {

	err := utils.PassLeadership(h.raft, h.db)
	if err != nil {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": err,
		})
	}

	return eCtx.JSON(http.StatusOK, map[string]interface{}{
		"message": "success transferring leader",
	})
}
