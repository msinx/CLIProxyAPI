package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/updatecheck"
)

type updateChecker interface {
	Check(context.Context) (updatecheck.Result, error)
}

func registerUpdateRoutes(router gin.IRoutes, checker updateChecker) {
	if checker == nil {
		checker = updatecheck.DefaultChecker()
	}

	router.GET("/update/check", func(c *gin.Context) {
		result, err := checker.Check(c.Request.Context())
		if err != nil {
			writeInternalError(c, "update check failed", err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
