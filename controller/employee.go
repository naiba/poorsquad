package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/naiba/poorsquad/model"
	"github.com/naiba/poorsquad/service/dao"
	"github.com/naiba/poorsquad/service/github"
	GitHubService "github.com/naiba/poorsquad/service/github"
)

// EmployeeController ..
type EmployeeController struct {
}

// ServeEmployee ..
func ServeEmployee(r gin.IRoutes) {
	ec := EmployeeController{}
	r.POST("/employee", ec.addOrEdit)
	r.DELETE("/employee/:what/:id/:userID", ec.remove)
}

type employeeForm struct {
	Type       string `binding:"required" json:"type,omitempty"`
	ID         uint64 `binding:"required,min=1" json:"id,omitempty"`
	Username   string `binding:"required" json:"username,omitempty"`
	Permission uint64 `json:"permission,omitempty"`
}

func (ec *EmployeeController) addOrEdit(c *gin.Context) {
	var ef employeeForm
	if err := c.ShouldBindJSON(&ef); err != nil {
		c.JSON(http.StatusOK, model.Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("格式错误：%s", err),
		})
		return
	}

	var company model.Company
	var account model.Account
	var team model.Team
	var user model.User
	var repository model.Repository
	var teamsID []uint64
	var err error

	switch ef.Type {
	case "company":
		ef.Permission--
		err = dao.DB.Where("id = ?", ef.ID).First(&company).Error
	case "team":
		err = dao.DB.Where("id = ?", ef.ID).First(&team).Error
		if err == nil {
			err = dao.DB.Where("id = ?", team.CompanyID).First(&company).Error
		}
	case "repository":
		err = dao.DB.Where("id = ?", ef.ID).First(&repository).Error
		if err == nil {
			var teamRepositories []model.TeamRepository
			err = dao.DB.Where("repository_id = ?", repository.ID).Find(&teamRepositories).Error
			if err == nil && len(teamRepositories) > 0 {
				for i := 0; i < len(teamRepositories); i++ {
					teamsID = append(teamsID, teamRepositories[i].TeamID)
				}
			}
			err = dao.DB.Where("id = ?", repository.AccountID).First(&account).Error
			if err == nil {
				err = dao.DB.Where("id = ?", account.CompanyID).First(&company).Error
			}
		}
	default:
		c.JSON(http.StatusOK, model.Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("错误：%s", "不支持的操作"),
		})
		return
	}

	if ef.Permission < 1 && ef.Type != "repository" {
		err = fmt.Errorf("没有这个权限：%d", ef.Permission)
	}

	if err == nil {
		err = dao.DB.Where("login = ?", ef.Username).First(&user).Error
		if err != nil {
			err = errors.New("为防止滥用，只允许主动登录过系统的用户被添加")
		}
	}

	if err != nil {
		c.JSON(http.StatusOK, model.Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("出现错误：%s", err),
		})
		return
	}

	// 验证管理权限
	u := c.MustGet(model.CtxKeyAuthorizedUser).(*model.User)
	companyPerm, errCompanyAdmin := company.CheckUserPermission(dao.DB, u.ID, model.UCPManager)
	teamPerm, errTeamAdmin := team.CheckUserPermission(dao.DB, u.ID, model.UTPManager)
	switch ef.Type {
	case "company":
		if errCompanyAdmin != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", errCompanyAdmin),
			})
			return
		}
		if companyPerm <= ef.Permission && companyPerm < model.UCPSuperManager {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "您只能授予低于自身的权限"),
			})
			return
		}
		var userCompany model.UserCompany
		userCompany.CompanyID = company.ID
		userCompany.UserID = user.ID
		userCompany.Permission = ef.Permission
		err = dao.DB.Save(&userCompany).Error
	case "team":
		if errCompanyAdmin != nil && errTeamAdmin != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", errTeamAdmin),
			})
			return
		}
		if errCompanyAdmin != nil && teamPerm <= ef.Permission {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "授权不能高于您自身权限"),
			})
			return
		}
		if errs := github.AddEmployeeToTeam(&team, &user, ef.Permission); errs != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusOK,
				Message: fmt.Sprintf("GitHub 同步：%s", errs),
			})
			return
		}
	case "repository":
		if errTeamAdmin != nil && errCompanyAdmin != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "权限不足"),
			})
			return
		}

		ctx := context.Background()
		client := GitHubService.NewAPIClient(ctx, account.Token)
		if err := github.AddEmployeeToRepository(ctx, client, &account, &repository, &user); err != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("GitHub 同步：%s", err),
			})
			return
		}
		github.RepositorySync(ctx, client, &account, &repository)
	}

	c.JSON(http.StatusOK, model.Response{
		Code: http.StatusOK,
	})
}

func (ec *EmployeeController) remove(c *gin.Context) {
	what := c.Param("what")
	id := c.Param("id")
	userID := c.Param("userID")

	var company model.Company
	var team model.Team
	var repository model.Repository
	var user model.User
	var account model.Account
	var teamsID []uint64
	var err error

	switch what {
	case "company":
		err = dao.DB.Where("id = ?", id).First(&company).Error
	case "team":
		err = dao.DB.Where("id = ?", id).First(&team).Error
		if err == nil {
			err = dao.DB.Where("id = ?", team.CompanyID).First(&company).Error
		}
	case "repository":
		err = dao.DB.Where("id = ?", id).First(&repository).Error
		if err == nil {
			var teamRepositories []model.TeamRepository
			err = dao.DB.Where("repository_id = ?", repository.ID).Find(&teamRepositories).Error
			if err == nil && len(teamRepositories) > 0 {
				for i := 0; i < len(teamRepositories); i++ {
					teamsID = append(teamsID, teamRepositories[i].TeamID)
				}
			}
			err = dao.DB.Where("id = ?", repository.AccountID).First(&account).Error
			if err == nil {
				err = dao.DB.Where("id = ?", account.CompanyID).First(&company).Error
			}
		}
	default:
		c.JSON(http.StatusOK, model.Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("错误：%s", "不支持的操作"),
		})
		return
	}

	if err == nil {
		err = dao.DB.Where("id = ?", userID).First(&user).Error
	}

	if err != nil {
		c.JSON(http.StatusOK, model.Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("数据库错误：%s", err),
		})
		return
	}

	// 验证管理权限
	u := c.MustGet(model.CtxKeyAuthorizedUser).(*model.User)
	myCompPerm, errCompanyAdmin := company.CheckUserPermission(dao.DB, u.ID, model.UCPManager)
	myTeamPerm, errTeamAdmin := team.CheckUserPermission(dao.DB, u.ID, model.UTPManager)
	switch what {
	case "company":
		userPerm, err := company.CheckUserPermission(dao.DB, user.ID, 0)
		if errCompanyAdmin != nil || (err != nil || (userPerm >= myCompPerm && myCompPerm != model.UCPSuperManager)) {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "权限不足"),
			})
			return
		}
		if err := dao.DB.Delete(model.UserCompany{}, "user_id = ? AND company_id = ?", user.ID, company.ID).Error; err != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusInternalServerError,
				Message: fmt.Sprintf("数据库错误：%s", err),
			})
			return
		}
	case "team":
		if errCompanyAdmin != nil && errTeamAdmin != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "权限不足"),
			})
			return
		}
		userPerm, err := team.CheckUserPermission(dao.DB, user.ID, 0)
		if errCompanyAdmin != nil && (err != nil || myTeamPerm < userPerm) {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "权限不足"),
			})
			return
		}
		team.FetchRepositoriesID(dao.DB)
		user.FetchTeams(dao.DB)
		if errs := github.RemoveEmployeeFromTeam(&team, &user); errs != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusOK,
				Message: fmt.Sprintf("GitHub 同步：%s", errs),
			})
			return
		}

	case "repository":
		if errCompanyAdmin != nil && errTeamAdmin != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("访问受限：%s", "权限不足"),
			})
			return
		}
		user.FetchTeams(dao.DB)
		ctx := context.Background()
		client := github.NewAPIClient(ctx, account.Token)
		if err := github.RemoveEmployeeFromRepository(ctx, client, &account, &repository, &user); err != nil {
			c.JSON(http.StatusOK, model.Response{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("GitHub 同步：%s", err),
			})
			return
		}
		github.RepositorySync(ctx, client, &account, &repository)
	}

	c.JSON(http.StatusOK, model.Response{
		Code: http.StatusOK,
	})
}
