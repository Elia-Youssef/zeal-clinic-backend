package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Public endpoints (no auth)

// GetPublicServices returns ZEAL clinic procedure categories and services
func GetPublicServices(c echo.Context) error {
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: zealServiceCatalog})
}

// GetBookingAvailability returns available timeslots for a given date
func GetBookingAvailability(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		log.Println("Error: GetBookingAvailability date query param missing")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "date query param required (YYYY-MM-DD)"})
	}

	// Get already-booked slots for this date
	bookedSlots, err := (&models.Booking{}).GetBookedSlots(date)
	if err != nil {
		log.Println("Error: GetBookingAvailability failed to check availability")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to check availability"})
	}
	bookedSet := make(map[string]bool)
	for _, s := range bookedSlots {
		bookedSet[s] = true
	}

	// Generate 30-min slots from 09:00 to 17:30
	type Slot struct {
		Time      string `json:"time"`
		Available bool   `json:"available"`
	}
	var slots []Slot
	for hour := 9; hour < 18; hour++ {
		for _, min := range []int{0, 30} {
			t := fmt.Sprintf("%02d:%02d", hour, min)
			slots = append(slots, Slot{Time: t, Available: !bookedSet[t]})
		}
	}

	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: slots})
}

// CheckBookingPatient checks if a patient exists by phone number
func CheckBookingPatient(c echo.Context) error {
	phone := c.QueryParam("phone")
	if phone == "" {
		log.Println("Error: CheckBookingPatient phone query param missing")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "phone query param required"})
	}
	found, firstName, err := (&models.Booking{}).PatientCheckByPhone(phone)
	if err != nil {
		log.Println("Error: CheckBookingPatient check failed")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "check failed"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: map[string]interface{}{
		"found":     found,
		"firstName": firstName,
	}})
}

// CreatePublicBooking creates a new booking from the public client flow
func CreatePublicBooking(c echo.Context) error {
	var b models.Booking
	if err := c.Bind(&b); err != nil {
		log.Println("Error: CreatePublicBooking invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("clientName", b.ClientName, "Client name")
	v.MinLength("clientName", b.ClientName, 2, "Client name")
	v.Required("clientPhone", b.ClientPhone, "Phone")
	v.Phone("clientPhone", b.ClientPhone)
	v.Email("clientEmail", b.ClientEmail)
	v.Required("serviceCategory", b.ServiceCategory, "Service category")
	v.Required("serviceName", b.ServiceName, "Service name")
	v.Required("preferredDate", b.PreferredDate, "Preferred date")
	v.Date("preferredDate", b.PreferredDate)
	v.Required("preferredTime", b.PreferredTime, "Preferred time")
	if v.HasErrors() {
		log.Println("Error: CreatePublicBooking validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	if err := b.Create(); err != nil {
		// Check if it's a booking limit error
		if strings.Contains(err.Error(), "booking limit reached") {
			log.Println("Error: CreatePublicBooking booking limit reached")
			return c.JSON(http.StatusTooManyRequests, utils.Response{Error: err.Error()})
		}
		log.Println("Error: CreatePublicBooking failed to create booking")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create booking"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: b})
}

// Staff endpoints (auth required)

func GetAllBookings(c echo.Context) error {
	status := c.QueryParam("status")
	date := c.QueryParam("date")

	bookings, err := (&models.Booking{}).GetAll(status, date)
	if err != nil {
		log.Println("Error: GetAllBookings failed to fetch bookings")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch bookings"})
	}
	if bookings == nil {
		bookings = []models.Booking{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: bookings})
}

// Booking confirmation converts a public booking into a real appointment.
// Flow: staff selects a room, then repo.Confirm() updates booking status to "confirmed", sets roomId,
// and creates a corresponding appointment record in the appointments table.
// This is the bridge between the public-facing booking flow and the internal scheduling system.
func ConfirmBooking(c echo.Context) error {
	var body struct {
		RoomID string `json:"roomId"`
	}
	if err := c.Bind(&body); err != nil || body.RoomID == "" {
		log.Println("Error: ConfirmBooking roomId is required")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "roomId is required"})
	}

	booking := models.Booking{ID: c.Param("id")}
	if err := booking.Confirm(body.RoomID); err == sql.ErrNoRows {
		log.Println("Error: ConfirmBooking booking not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "booking not found"})
	} else if err != nil {
		log.Println("Error: ConfirmBooking " + err.Error())
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: booking})
}

func CancelBooking(c echo.Context) error {
	booking := models.Booking{ID: c.Param("id")}
	if err := booking.Cancel(); err == sql.ErrNoRows {
		log.Println("Error: CancelBooking booking not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "booking not found"})
	} else if err != nil {
		log.Println("Error: CancelBooking failed to cancel booking")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to cancel booking"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: booking})
}

// ZEAL Service Catalog (hardcoded, clinic procedures only)

type zealService struct {
	Name        string `json:"name"`
	Price       int    `json:"price"`
	Subcategory string `json:"subcategory"`
}

type zealCategory struct {
	Category string        `json:"category"`
	Services []zealService `json:"services"`
}

var zealServiceCatalog = []zealCategory{
	{Category: "Botox", Services: []zealService{
		{Name: "Botox Full", Price: 220, Subcategory: "Women"},
		{Name: "Botox Full (Dysport)", Price: 250, Subcategory: "Women"},
		{Name: "Botox Full + Bunny Lines", Price: 250, Subcategory: "Women"},
		{Name: "Botox Marionette Lines (DAO)", Price: 30, Subcategory: "Women"},
		{Name: "Botox Around Eyes", Price: 120, Subcategory: "Women"},
		{Name: "Botox Frown Lines", Price: 120, Subcategory: "Women"},
		{Name: "Botox Gummy Smile", Price: 120, Subcategory: "Women"},
		{Name: "Botox Clenching Teeth", Price: 350, Subcategory: "Women"},
		{Name: "Botox Sweating", Price: 350, Subcategory: "Women"},
		{Name: "Botox Migraine", Price: 400, Subcategory: "Women"},
		{Name: "Botox Neck", Price: 220, Subcategory: "Women"},
		{Name: "Botox Calves", Price: 350, Subcategory: "Women"},
		{Name: "Botox Jaw", Price: 150, Subcategory: "Women"},
		{Name: "Traptox", Price: 350, Subcategory: "Women"},
		{Name: "Lip Flip", Price: 120, Subcategory: "Women"},
		{Name: "Botox Full", Price: 250, Subcategory: "Men"},
		{Name: "Botox Full (Dysport)", Price: 280, Subcategory: "Men"},
		{Name: "Botox Full + Bunny Lines", Price: 280, Subcategory: "Men"},
		{Name: "Botox Marionette Lines (DAO)", Price: 30, Subcategory: "Men"},
		{Name: "Botox Around Eyes", Price: 150, Subcategory: "Men"},
		{Name: "Botox Frown Lines", Price: 150, Subcategory: "Men"},
		{Name: "Botox Gummy Smile", Price: 120, Subcategory: "Men"},
		{Name: "Botox Teeth Clenching", Price: 350, Subcategory: "Men"},
		{Name: "Botox Sweating", Price: 350, Subcategory: "Men"},
		{Name: "Botox Migraine", Price: 400, Subcategory: "Men"},
		{Name: "Botox Neck", Price: 250, Subcategory: "Men"},
		{Name: "Botox Calves", Price: 350, Subcategory: "Men"},
		{Name: "Botox Jaw", Price: 180, Subcategory: "Men"},
		{Name: "Traptox", Price: 350, Subcategory: "Men"},
		{Name: "Lip Flip", Price: 120, Subcategory: "Men"},
	}},
	{Category: "Fillers", Services: []zealService{
		{Name: "Lips", Price: 250, Subcategory: "Face"},
		{Name: "Nasolabial Folds", Price: 350, Subcategory: "Face"},
		{Name: "Marionette Lines", Price: 350, Subcategory: "Face"},
		{Name: "Cheeks", Price: 350, Subcategory: "Face"},
		{Name: "Jawline", Price: 350, Subcategory: "Face"},
		{Name: "Chin", Price: 300, Subcategory: "Face"},
		{Name: "Under Eyes", Price: 350, Subcategory: "Face"},
		{Name: "Nose (Non-Surgical)", Price: 350, Subcategory: "Face"},
		{Name: "Temples", Price: 350, Subcategory: "Face"},
		{Name: "Earlobes", Price: 200, Subcategory: "Face"},
		{Name: "Hands", Price: 350, Subcategory: "Body"},
	}},
	{Category: "Skin Boosters", Services: []zealService{
		{Name: "Profhilo Face", Price: 250, Subcategory: "Face"},
		{Name: "Profhilo Neck", Price: 250, Subcategory: "Face"},
		{Name: "Skinvive", Price: 250, Subcategory: "Face"},
		{Name: "Sculptra", Price: 350, Subcategory: "Face"},
		{Name: "Exosome", Price: 350, Subcategory: "Face"},
		{Name: "Collagen Booster", Price: 250, Subcategory: "Face"},
		{Name: "Jalupro Face", Price: 200, Subcategory: "Face"},
		{Name: "Jalupro Neck", Price: 200, Subcategory: "Face"},
		{Name: "Profhilo Body", Price: 350, Subcategory: "Body"},
		{Name: "Jalupro Eye", Price: 200, Subcategory: "Eyes"},
		{Name: "Chroma Phill Art Eye", Price: 200, Subcategory: "Eyes"},
	}},
	{Category: "Morpheus8", Services: []zealService{
		{Name: "Face + Plasma", Price: 350, Subcategory: "Face"},
		{Name: "Neck + Plasma", Price: 250, Subcategory: "Face"},
		{Name: "Face & Neck + Plasma", Price: 500, Subcategory: "Face"},
		{Name: "Arms", Price: 350, Subcategory: "Body"},
		{Name: "Abdominal", Price: 500, Subcategory: "Body"},
		{Name: "Love Handles", Price: 350, Subcategory: "Body"},
		{Name: "Thighs", Price: 500, Subcategory: "Body"},
		{Name: "Buttocks", Price: 500, Subcategory: "Body"},
		{Name: "Hands", Price: 250, Subcategory: "Body"},
		{Name: "Knees", Price: 250, Subcategory: "Body"},
	}},
	{Category: "Laser Hair Removal", Services: []zealService{
		{Name: "Full Face", Price: 60, Subcategory: "Single Areas"},
		{Name: "Upper Lip", Price: 15, Subcategory: "Single Areas"},
		{Name: "Sideburns", Price: 20, Subcategory: "Single Areas"},
		{Name: "Chin", Price: 15, Subcategory: "Single Areas"},
		{Name: "Full Arms", Price: 60, Subcategory: "Single Areas"},
		{Name: "Half Arms", Price: 40, Subcategory: "Single Areas"},
		{Name: "Underarms", Price: 25, Subcategory: "Single Areas"},
		{Name: "Full Legs", Price: 80, Subcategory: "Single Areas"},
		{Name: "Half Legs", Price: 50, Subcategory: "Single Areas"},
		{Name: "Bikini", Price: 40, Subcategory: "Single Areas"},
		{Name: "Brazilian", Price: 60, Subcategory: "Single Areas"},
		{Name: "Chest", Price: 50, Subcategory: "Single Areas"},
		{Name: "Abdomen", Price: 50, Subcategory: "Single Areas"},
		{Name: "Full Back", Price: 60, Subcategory: "Single Areas"},
		{Name: "Half Back", Price: 40, Subcategory: "Single Areas"},
		{Name: "Neck", Price: 25, Subcategory: "Single Areas"},
		{Name: "Buttocks", Price: 40, Subcategory: "Single Areas"},
		{Name: "Hands / Feet", Price: 20, Subcategory: "Single Areas"},
		{Name: "Full Body Package 1", Price: 200, Subcategory: "Packages"},
		{Name: "Full Body Package 2", Price: 250, Subcategory: "Packages"},
		{Name: "Full Body Package 3", Price: 300, Subcategory: "Packages"},
	}},
	{Category: "Quanta Machine", Services: []zealService{
		{Name: "Tattoo Removal — Eyebrows", Price: 100, Subcategory: "Tattoo Removal"},
		{Name: "Tattoo Removal — Face/Body", Price: 150, Subcategory: "Tattoo Removal"},
		{Name: "Varicose Vein — Per Vein", Price: 50, Subcategory: "Varicose"},
		{Name: "Varicose — Full Face", Price: 150, Subcategory: "Varicose"},
		{Name: "Varicose — Full Body", Price: 300, Subcategory: "Varicose"},
		{Name: "Melasma Q-Switched", Price: 100, Subcategory: "Melasma"},
		{Name: "Melasma Full Face", Price: 200, Subcategory: "Melasma"},
		{Name: "Carbon Peel", Price: 100, Subcategory: "Other"},
		{Name: "Hair Bleaching", Price: 100, Subcategory: "Other"},
		{Name: "Rosacea Treatment", Price: 150, Subcategory: "Other"},
		{Name: "Scar Treatment", Price: 100, Subcategory: "Other"},
		{Name: "Cherry Angiomas", Price: 50, Subcategory: "Other"},
		{Name: "Freckles Removal", Price: 100, Subcategory: "Other"},
	}},
	{Category: "CO2 Laser", Services: []zealService{
		{Name: "Single Session", Price: 200, Subcategory: "Sessions"},
		{Name: "3-Session Package", Price: 500, Subcategory: "Sessions"},
		{Name: "Under Eyes", Price: 150, Subcategory: "Face"},
		{Name: "Full Face", Price: 300, Subcategory: "Face"},
		{Name: "Neck", Price: 200, Subcategory: "Face"},
		{Name: "Body Area", Price: 250, Subcategory: "Body"},
	}},
	{Category: "Creams", Services: []zealService{
		{Name: "Sodermix", Price: 30, Subcategory: "Standard"},
		{Name: "Keloplast", Price: 25, Subcategory: "Standard"},
		{Name: "Beclean", Price: 20, Subcategory: "Standard"},
		{Name: "Boost C", Price: 35, Subcategory: "Standard"},
		{Name: "Boost Eye", Price: 30, Subcategory: "Standard"},
		{Name: "Boost Glow", Price: 35, Subcategory: "Standard"},
		{Name: "Boost Lift", Price: 35, Subcategory: "Standard"},
		{Name: "Boost Mat", Price: 30, Subcategory: "Standard"},
		{Name: "Boost Relax", Price: 30, Subcategory: "Standard"},
		{Name: "Hair Care Serum", Price: 40, Subcategory: "Pharmaceries"},
		{Name: "Retinol Serum", Price: 35, Subcategory: "Pharmaceries"},
		{Name: "Whitening Cream", Price: 30, Subcategory: "Pharmaceries"},
	}},
	{Category: "Others", Services: []zealService{
		{Name: "Filler Dissolver", Price: 150, Subcategory: "General"},
		{Name: "Triple Enzymes", Price: 100, Subcategory: "General"},
		{Name: "PRP (Platelet-Rich Plasma)", Price: 200, Subcategory: "General"},
		{Name: "Consultation with Dr. Joe", Price: 50, Subcategory: "General"},
	}},
}
