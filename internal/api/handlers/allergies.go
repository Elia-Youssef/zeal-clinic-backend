package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func GetAllAllergies(c echo.Context) error {
	items, err := (&models.Allergy{}).GetAll()
	if err != nil {
		log.Println("Error: [GetAllAllergies] failed to fetch allergies:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch allergies"})
	}
	if items == nil {
		items = []models.Allergy{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func CreateAllergy(c echo.Context) error {
	var a models.Allergy
	if err := c.Bind(&a); err != nil {
		log.Println("Error: [CreateAllergy] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("name", a.Name, "Name")
	if v.HasErrors() {
		log.Println("Error: [CreateAllergy] validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	a.CreatedAt = time.Now().Format(time.RFC3339)
	if err := a.Create(); err != nil {
		log.Println("Error: [CreateAllergy] failed to create allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create allergy"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: a})
}

func UpdateAllergy(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdateAllergy] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	a := models.Allergy{ID: c.Param("id")}
	if err := a.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [UpdateAllergy] allergy not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "allergy not found"})
		}
		log.Println("Error: [UpdateAllergy] failed to update allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: a})
}

func DeleteAllergy(c echo.Context) error {
	a := models.Allergy{ID: c.Param("id")}
	if err := a.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [DeleteAllergy] allergy not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "allergy not found"})
		}
		log.Println("Error: [DeleteAllergy] failed to delete allergy:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Patient Allergies
func GetPatientAllergies(c echo.Context) error {
	items, err := (&models.PatientAllergy{}).GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetPatientAllergies] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient allergies"})
	}
	if items == nil {
		items = []models.PatientAllergy{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddPatientAllergy(c echo.Context) error {
	var pa models.PatientAllergy
	if err := c.Bind(&pa); err != nil {
		log.Println("Error: [AddPatientAllergy] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pa.PatientID = c.Param("patientId")
	v := NewValidator()
	v.Required("allergyId", pa.AllergyID, "Allergy ID")
	if v.HasErrors() {
		log.Println("Error: [AddPatientAllergy] validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	pa.CreatedAt = time.Now().Format(time.RFC3339)
	if err := pa.Create(); err != nil {
		log.Println("Error: [AddPatientAllergy] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add patient allergy"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pa})
}

func RemovePatientAllergy(c echo.Context) error {
	pa := models.PatientAllergy{ID: c.Param("id")}
	if err := pa.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemovePatientAllergy] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "patient allergy not found"})
		}
		log.Println("Error: [RemovePatientAllergy] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove patient allergy"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Procedure allergy conflicts
func GetProcedureAllergyConflicts(c echo.Context) error {
	items, err := (&models.ProcedureAllergyConflict{}).GetByProcedure(c.Param("procedureId"))
	if err != nil {
		log.Println("Error: [GetProcedureAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch procedure allergy conflicts"})
	}
	if items == nil {
		items = []models.ProcedureAllergyConflict{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddProcedureAllergyConflict(c echo.Context) error {
	var pac models.ProcedureAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pac.ProcedureID = c.Param("procedureId")
	v := NewValidator()
	v.Required("allergyId", pac.AllergyID, "Allergy ID")
	if v.HasErrors() {
		log.Println("Error: [AddProcedureAllergyConflict] validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	if pac.Severity == "" {
		pac.Severity = "warning"
	}
	pac.CreatedAt = time.Now().Format(time.RFC3339)
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProcedureAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add procedure allergy conflict"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pac})
}

func RemoveProcedureAllergyConflict(c echo.Context) error {
	pac := models.ProcedureAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemoveProcedureAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "conflict not found"})
		}
		log.Println("Error: [RemoveProcedureAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove conflict"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Product allergy conflicts
func GetProductAllergyConflicts(c echo.Context) error {
	items, err := (&models.ProductAllergyConflict{}).GetBySKU(c.Param("sku"))
	if err != nil {
		log.Println("Error: [GetProductAllergyConflicts] failed to fetch:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch product allergy conflicts"})
	}
	if items == nil {
		items = []models.ProductAllergyConflict{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func AddProductAllergyConflict(c echo.Context) error {
	var pac models.ProductAllergyConflict
	if err := c.Bind(&pac); err != nil {
		log.Println("Error: [AddProductAllergyConflict] invalid request body")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	pac.SKU = c.Param("sku")
	v := NewValidator()
	v.Required("allergyId", pac.AllergyID, "Allergy ID")
	if v.HasErrors() {
		log.Println("Error: [AddProductAllergyConflict] validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	if pac.Severity == "" {
		pac.Severity = "warning"
	}
	pac.CreatedAt = time.Now().Format(time.RFC3339)
	if err := pac.Create(); err != nil {
		log.Println("Error: [AddProductAllergyConflict] failed to add:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add product allergy conflict"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: pac})
}

func RemoveProductAllergyConflict(c echo.Context) error {
	pac := models.ProductAllergyConflict{ID: c.Param("id")}
	if err := pac.Delete(); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: [RemoveProductAllergyConflict] not found:", c.Param("id"))
			return c.JSON(http.StatusNotFound, utils.Response{Error: "conflict not found"})
		}
		log.Println("Error: [RemoveProductAllergyConflict] failed to remove:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove conflict"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
