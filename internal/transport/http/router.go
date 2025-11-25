package http

import (
	"github.com/gin-gonic/gin"
	"github.com/n1ckerr0r/pull-requests-service/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewRouter(s *store.Store) *gin.Engine {
	h := &Handler{store: s}
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// Teams
	r.POST("/team/add", h.HandleTeamAdd)
	r.GET("/team/get", h.HandleTeamGet)

	// Users
	r.POST("/users/setIsActive", h.HandleSetIsActive)
	r.GET("/users/getReview", h.HandleGetReview)

	// PRs
	r.POST("/pullRequest/create", h.HandleCreatePR)
	r.POST("/pullRequest/merge", h.HandleMergePR)
	r.POST("/pullRequest/reassign", h.HandleReassign)

	return r
}
