package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllTeamMembers(c echo.Context) error {
	members, err := (&models.TeamMember{}).GetAll()
	if err != nil {
		log.Println("Error: GetAllTeamMembers failed to fetch team members")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch team members"})
	}
	if members == nil {
		members = []models.TeamMember{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: members})
}

func CreateTeamMember(c echo.Context) error {
	var m models.TeamMember
	if err := c.Bind(&m); err != nil {
		log.Println("Error: CreateTeamMember invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("firstName", m.FirstName, "First name")
	v.Required("lastName", m.LastName, "Last name")
	v.Required("role", m.Role, "Role")
	v.Required("contact", m.Contact, "Contact")
	v.Phone("contact", m.Contact)
	v.Email("email", m.Email)
	v.Required("employmentType", m.EmploymentType, "Employment type")
	v.OneOf("employmentType", m.EmploymentType, []string{"Full-time", "Part-time"}, "Employment type")
	v.Positive("salary", m.Salary, "Salary")
	v.Required("status", m.Status, "Status")
	v.OneOf("status", m.Status, []string{"Active", "Inactive"}, "Status")
	if v.HasErrors() {
		log.Println("Error: CreateTeamMember validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	if m.Schedule == nil {
		m.Schedule = models.StringSlice{}
	}
	if m.OffDays == nil {
		m.OffDays = models.StringSlice{}
	}

	if err := m.Create(); err != nil {
		log.Println("Error: CreateTeamMember failed to create team member")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create team member"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: m})
}

func UpdateTeamMember(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateTeamMember invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")

	member := models.TeamMember{ID: c.Param("id")}
	if err := member.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateTeamMember team member not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "team member not found"})
	} else if err != nil {
		log.Println("Error: UpdateTeamMember failed to update team member")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update team member"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: member})
}

func DeleteTeamMember(c echo.Context) error {
	member := models.TeamMember{ID: c.Param("id")}
	if err := member.Delete(); err == sql.ErrNoRows {
		log.Println("Error: DeleteTeamMember team member not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "team member not found"})
	} else if err != nil {
		log.Println("Error: DeleteTeamMember failed to delete team member")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete team member"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
