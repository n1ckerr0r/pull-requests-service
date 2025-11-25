package http

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/n1ckerr0r/pull-requests-service/internal/domain"
	"github.com/n1ckerr0r/pull-requests-service/internal/store"
)

var (
	ErrReviewerNotAssigned = errors.New("reviewer not assigned")
	ErrNoCandidate         = errors.New("no candidate")
)

type TeamMemberDTO struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type TeamDTO struct {
	TeamName string          `json:"team_name"`
	Members  []TeamMemberDTO `json:"members"`
}

func (h *Handler) HandleTeamAdd(c *gin.Context) {
	var req TeamDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	if err := h.store.CreateTeam(c.Request.Context(), &domain.Team{Name: req.TeamName}); err != nil {
		if err == store.ErrAlreadyExists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "TEAM_EXISTS",
					"message": "team_name already exists",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	// upsert members
	for _, m := range req.Members {
		u := domain.NewUser(m.UserID, m.Username, domain.TeamID(req.TeamName), m.IsActive)
		if createErr := h.store.UpsertUser(c.Request.Context(), u); createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL",
					"message": createErr.Error(),
				},
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"team": req})
}

func (h *Handler) HandleTeamGet(c *gin.Context) {
	teamName := c.Query("team_name")
	if teamName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": "team_name required",
			},
		})
		return
	}

	team, members, err := h.store.GetTeam(c.Request.Context(), teamName)
	if err != nil {
		if err == store.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "team not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	respMembers := make([]TeamMemberDTO, 0, len(members))
	for _, m := range members {
		respMembers = append(respMembers, TeamMemberDTO{
			UserID:   string(m.ID),
			Username: m.Username,
			IsActive: m.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"team_name": team.Name,
		"members":   respMembers,
	})
}

func (h *Handler) HandleSetIsActive(c *gin.Context) {
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	u, err := h.store.SetUserActive(c.Request.Context(), req.UserID, req.IsActive)
	if err != nil {
		if err == store.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "user not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"user_id":   u.ID,
			"username":  u.Username,
			"team_name": u.TeamName,
			"is_active": u.IsActive,
		},
	})
}

func (h *Handler) HandleCreatePR(c *gin.Context) {
	var req struct {
		PRID   string `json:"pull_request_id"`
		Name   string `json:"pull_request_name"`
		Author string `json:"author_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	author, err := h.store.GetUser(c.Request.Context(), req.Author)
	if err != nil {
		if err == store.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "author not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	pr := domain.NewPR(req.PRID, req.Name, domain.UserID(req.Author))

	if createErr := h.store.CreatePR(c.Request.Context(), pr); createErr != nil {
		if createErr == store.ErrAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "PR_EXISTS",
					"message": "PR id already exists",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": createErr.Error(),
			},
		})
		return
	}

	var candidates []string
	selectErr := h.store.DB().SelectContext(c.Request.Context(), &candidates,
		`SELECT user_id FROM users WHERE team_name = $1 AND is_active = true AND user_id <> $2`,
		author.TeamName, req.Author,
	)
	if selectErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": selectErr.Error(),
			},
		})
		return
	}

	selected := pickRandomUpTo(candidates, 2)
	if len(selected) > 0 {
		if assignErr := h.store.AssignReviewers(c.Request.Context(), pr.ID, selected); assignErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL",
					"message": assignErr.Error(),
				},
			})
			return
		}
	}

	created, getErr := h.store.GetPR(c.Request.Context(), pr.ID)
	if getErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": getErr.Error(),
			},
		})
		return
	}

	ar := make([]string, 0, len(created.AssignedReviewers))
	for _, r := range created.AssignedReviewers {
		ar = append(ar, string(r))
	}

	c.JSON(http.StatusCreated, gin.H{
		"pr": gin.H{
			"pull_request_id":    created.ID,
			"pull_request_name":  created.Name,
			"author_id":          created.AuthorID,
			"status":             created.Status,
			"assigned_reviewers": ar,
			"createdAt":          created.CreatedAt,
		},
	})
}

func pickRandomUpTo(candidates []string, k int) []string {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) <= k {
		return candidates
	}
	return candidates[:k]
}

func (h *Handler) HandleMergePR(c *gin.Context) {
	var req struct {
		PRID string `json:"pull_request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": err.Error(),
			},
		})
		return
	}

	pr, err := h.store.SetPRMerged(c.Request.Context(), req.PRID)
	if err != nil {
		if err == store.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "NOT_FOUND",
					"message": "pr not found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	ar := make([]string, 0, len(pr.AssignedReviewers))
	for _, r := range pr.AssignedReviewers {
		ar = append(ar, string(r))
	}

	c.JSON(http.StatusOK, gin.H{
		"pr": gin.H{
			"pull_request_id":    pr.ID,
			"pull_request_name":  pr.Name,
			"author_id":          pr.AuthorID,
			"status":             pr.Status,
			"assigned_reviewers": ar,
			"mergedAt":           pr.MergedAt,
		},
	})
}

func (h *Handler) HandleReassign(c *gin.Context) {
	var req struct {
		PRID    string `json:"pull_request_id"`
		OldUser string `json:"old_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "BAD_REQUEST", "message": err.Error()},
		})
		return
	}

	candidate, err := h.store.ReassignReviewer(c.Request.Context(), req.PRID, req.OldUser)
	if err != nil {
		if errors.Is(err, store.ErrReviewerNotAssigned) {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{"code": "NOT_ASSIGNED", "message": "reviewer is not assigned to this PR"},
			})
			return
		}
		if errors.Is(err, store.ErrNoCandidate) {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{"code": "NO_CANDIDATE", "message": "no active replacement candidate in team"},
			})
			return
		}
		if errors.Is(err, store.ErrPRMerged) {
			c.JSON(http.StatusConflict, gin.H{
				"error": gin.H{"code": "PR_MERGED", "message": "cannot reassign on merged PR"},
			})
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{"code": "NOT_FOUND", "message": "pr or user not found"},
			})
			return
		}

		log.Printf("internal error reassign (%s, %s): %v", req.PRID, req.OldUser, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "INTERNAL", "message": "internal server error"},
		})
		return
	}

	updated, err := h.store.GetPR(c.Request.Context(), req.PRID)
	if err != nil {
		log.Printf("failed to fetch updated PR after reassign: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "INTERNAL", "message": "internal server error"},
		})
		return
	}

	ar := make([]string, 0, len(updated.AssignedReviewers))
	for _, r := range updated.AssignedReviewers {
		ar = append(ar, string(r))
	}

	c.JSON(http.StatusOK, gin.H{
		"pr": gin.H{
			"pull_request_id":    updated.ID,
			"pull_request_name":  updated.Name,
			"author_id":          updated.AuthorID,
			"status":             updated.Status,
			"assigned_reviewers": ar,
		},
		"replaced_by": candidate,
	})
}

func (h *Handler) HandleGetReview(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "BAD_REQUEST",
				"message": "user_id required",
			},
		})
		return
	}

	prs, err := h.store.GetPRsByReviewer(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": err.Error(),
			},
		})
		return
	}

	shorts := make([]gin.H, 0, len(prs))
	for _, p := range prs {
		shorts = append(shorts, gin.H{
			"pull_request_id":   p.ID,
			"pull_request_name": p.Name,
			"author_id":         p.AuthorID,
			"status":            p.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":       userID,
		"pull_requests": shorts,
	})
}
