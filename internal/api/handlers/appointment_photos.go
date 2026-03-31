package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func GetAppointmentPhotos(c echo.Context) error {
	items, err := (&models.AppointmentPhoto{}).GetByAppointmentID(c.Param("id"))
	if err != nil {
		log.Println("Error: GetAppointmentPhotos failed to fetch photos:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch photos"})
	}
	if items == nil {
		items = []models.AppointmentPhoto{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func UploadAppointmentPhoto(c echo.Context) error {
	appointmentID := c.Param("id")

	file, err := c.FormFile("photo")
	if err != nil {
		log.Println("Error: UploadAppointmentPhoto photo file required:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "photo file required"})
	}

	if file.Size > 10<<20 {
		log.Println("Error: UploadAppointmentPhoto file too large")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "file too large (max 10MB)"})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	if !allowed[ext] {
		log.Println("Error: UploadAppointmentPhoto file type not allowed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "file type not allowed"})
	}

	src, err := file.Open()
	if err != nil {
		log.Println("Error: UploadAppointmentPhoto failed to read file:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to read file"})
	}
	defer src.Close()

	os.MkdirAll("uploads", 0755)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	destPath := filepath.Join("uploads", filename)
	dst, err := os.Create(destPath)
	if err != nil {
		log.Println("Error: UploadAppointmentPhoto failed to save file:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to save file"})
	}
	defer dst.Close()
	io.Copy(dst, src)

	photo := models.AppointmentPhoto{
		AppointmentID: appointmentID,
		FilePath:      "/uploads/" + filename,
		Caption:       c.FormValue("caption"),
		CreatedAt:     models.DateNow(),
	}

	if err := photo.Create(); err != nil {
		log.Println("Error: UploadAppointmentPhoto failed to save photo record:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to save photo record"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: photo})
}

func DeleteAppointmentPhoto(c echo.Context) error {
	photo := models.AppointmentPhoto{ID: c.Param("photoId")}
	if err := photo.GetByID(photo.ID); err != nil {
		if err == sql.ErrNoRows {
			log.Println("Error: DeleteAppointmentPhoto photo not found:", err)
			return c.JSON(http.StatusNotFound, utils.Response{Error: "photo not found"})
		}
		log.Println("Error: DeleteAppointmentPhoto failed to find photo:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to find photo"})
	}

	// Delete file
	filePath := strings.TrimPrefix(photo.FilePath, "/")
	os.Remove(filePath)

	if err := photo.Delete(); err != nil {
		log.Println("Error: DeleteAppointmentPhoto failed to delete photo:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete photo"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
