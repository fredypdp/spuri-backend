package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCampoPresenteNoPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body string
		want bool
	}{{`{}`, false}, {`{"sumario_id":"x"}`, true}, {`{"sumario_id":null}`, true}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("PATCH", "/", strings.NewReader(tc.body))
		got, err := campoPresenteNoPayload(c, "sumario_id")
		if err != nil || got != tc.want {
			t.Errorf("%s got %v err %v", tc.body, got, err)
		}
	}
}
