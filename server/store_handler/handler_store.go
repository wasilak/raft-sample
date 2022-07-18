package store_handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"ysf/raftsample/fsm"

	"github.com/hashicorp/raft"
	"github.com/labstack/echo/v4"
)

// requestStore payload for storing new data in raft cluster
type requestStore struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// Store handling save to raft cluster. Store will invoke raft.Apply to make this stored in all cluster
// with acknowledge from n quorum. Store must be done in raft leader, otherwise return error.
func (h handler) Store(eCtx echo.Context) error {
	var form = requestStore{}
	if err := eCtx.Bind(&form); err != nil {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": fmt.Sprintf("error binding: %s", err.Error()),
		})
	}

	form.Key = strings.TrimSpace(form.Key)
	if form.Key == "" {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": "key is empty",
		})
	}

	if h.raft.State() != raft.Leader {
		postBody, _ := json.Marshal(form)
		responseBody := bytes.NewBuffer(postBody)

		url := fmt.Sprintf("http://%s/store", h.raft.Leader())

		fmt.Println(url)
		fmt.Printf("%+v", form)
		fmt.Println(postBody)

		resp, err := http.Post(url, "application/json", responseBody)

		//Handle Error
		if err != nil {
			return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
				"error": err,
			})
		}
		defer resp.Body.Close()

		return eCtx.JSON(http.StatusOK, map[string]interface{}{
			"message": "success persisting data",
			"data":    form,
		})

		// return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
		// 	"error":  "not the leader",
		// 	"leader": h.raft.Leader(),
		// })
	}

	configFuture := h.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": fmt.Sprintf("failed to get raft configuration: %s", err.Error()),
		})
	}

	fmt.Printf("%+v", configFuture)

	fmt.Printf("on leader: %+v", form)

	payload := fsm.CommandPayload{
		Operation: "SET",
		Key:       form.Key,
		Value:     form.Value,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": fmt.Sprintf("error preparing saving data payload: %s", err.Error()),
		})
	}

	applyFuture := h.raft.Apply(data, 500*time.Millisecond)
	if err := applyFuture.Error(); err != nil {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": fmt.Sprintf("error persisting data in raft cluster: %s", err.Error()),
		})
	}

	_, ok := applyFuture.Response().(*fsm.ApplyResponse)
	if !ok {
		return eCtx.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
			"error": fmt.Sprintf("error response is not match apply response"),
		})
	}

	return eCtx.JSON(http.StatusOK, map[string]interface{}{
		"message": "success persisting data",
		"data":    form,
	})
}
