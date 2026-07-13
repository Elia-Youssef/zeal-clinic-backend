package legacyimport

import (
	"time"

	"clinic-api/internal/legacyimport/conv"
	"clinic-api/internal/legacyimport/csvutil"
)

func mapStatus(s string) (mapped string, known bool) {
	switch s {
	case "Started", "Arrived":
		return "In-Progress", true
	case "Reserved", "Confirmed":
		return "Scheduled", true
	case "Cancelled":
		return "Cancelled", true
	case "Scheduled", "In-Progress", "Completed", "Rescheduled":
		return s, true
	default:
		return "Scheduled", false
	}
}

func resolveStatus(raw, endTime, nowZ string) (status string, known bool) {
	mapped, known := mapStatus(raw)
	if mapped == "Cancelled" {
		return "Cancelled", known
	}
	if endTime != "" && endTime < nowZ {
		return "Completed", known
	}
	return mapped, known
}

func migrateAppointments(c *Context, codeLabels map[string]string) (appts, apptProcs *rowset, err error) {
	f, err := c.File("appointments.csv")
	if err != nil {
		return nil, nil, err
	}
	sec := c.Report.Section("appointments")
	t := newRowset(
		"id", "patient_id", "room_id", "start_time", "end_time",
		"status", "notes", "cancel_notes", "completion_notes", "created_at")
	ap := newRowset("id", "patient_id", "procedure_id", "appointment_id", "created_at")

	col := func(n string) int { return f.Col(n) }
	var read, skipped, badStatus, badTime, backfilled, completedPast, linked int
	unknownStatuses := map[string]bool{}
	idSeen := map[string]bool{}
	nowZ := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	for _, row := range f.Rows {
		id := csvutil.Get(row, col("id"))
		if id == "" || id == "0" {
			continue
		}
		if idSeen[id] {
			sec.Issue(c.Report, "duplicate appointment id %q dropped.", id)
			continue
		}
		read++

		date := csvutil.Get(row, col("createdat"))
		start, sok, serr := conv.AppointmentDateTime(date, csvutil.Get(row, col("starttime")))
		end, eok, eerr := conv.AppointmentDateTime(date, csvutil.Get(row, col("endtime")))
		if serr != nil || eerr != nil || !sok || !eok {
			skipped++
			badTime++
			sec.Issue(c.Report, "appointment %q: bad date/time (date=%q start=%q end=%q) — skipped.",
				id, date, csvutil.Get(row, col("starttime")), csvutil.Get(row, col("endtime")))
			continue
		}
		idSeen[id] = true

		patientID := csvutil.Get(row, col("patientid"))
		if patientID == "" || patientID == "0" {
			skipped++
			sec.Issue(c.Report, "appointment %q has no patient id — skipped.", id)
			continue
		}
		if !c.knownPatient[patientID] {
			backfilled++
		}
		c.RegisterPatientRef(patientID, csvutil.Get(row, col("patientnam")))

		status, ok := resolveStatus(csvutil.Get(row, col("status")), end, nowZ)
		if !ok {
			badStatus++
			unknownStatuses[csvutil.Get(row, col("status"))] = true
		}
		if status == "Completed" {
			completedPast++
		}

		created := c.MigrationTS
		if ca, cok, _ := conv.DateTimeZ(date); cok {
			created = ca
		}

		t.add(
			id, patientID,
			c.resolveRoomID(csvutil.Get(row, col("room"))),
			start, end, status,
			appointmentNotes(codeLabels, row, col),
			csvutil.Get(row, col("cancelnote")),
			csvutil.Get(row, col("completion")),
			created,
		)

		if code := csvutil.Get(row, col("procedure")); code != "" {
			if procID := c.resolveCategoryProcedureID(codeLabels[code]); procID != "" {
				ap.add(c.ID(created, "appointment_procedure:"+id), patientID, procID, id, created)
				linked++
			}
		}
	}

	sec.Counts(read, t.len(), skipped)
	sec.Note("Past appointments (end time before now) -> Completed (%d). Cancelled stays Cancelled; still-future ones use the legacy mapping (Started/Arrived->In-Progress, Reserved/Confirmed->Scheduled).", completedPast)
	sec.Note("start_time/end_time = appointment date (createdat) + time; procedure code preserved in notes.")
	sec.Note("Linked %d appointment(s) to a (deprecated) category-level procedure via appointment_procedures.", linked)
	if backfilled > 0 {
		sec.Note("%d appointment(s) referenced a patient not in patients.csv (backfilled by name).", backfilled)
	}
	if badTime > 0 {
		sec.Issue(c.Report, "%d appointment(s) skipped due to unparseable date/time.", badTime)
	}
	if badStatus > 0 {
		list := make([]string, 0, len(unknownStatuses))
		for s := range unknownStatuses {
			list = append(list, s)
		}
		sec.Issue(c.Report, "%d appointment(s) had an unrecognized status (treated as Completed if past, else Scheduled): %v.", badStatus, list)
	}
	return t, ap, nil
}

func appointmentNotes(codeLabels map[string]string, row []string, col func(string) int) string {
	notes := csvutil.Get(row, col("notes"))
	code := csvutil.Get(row, col("procedure"))
	if code == "" {
		return notes
	}
	label := "Procedure: " + code
	if name := codeLabels[code]; name != "" {
		label = "Procedure: " + name + " (" + code + ")"
	}
	if notes == "" {
		return label
	}
	return label + " | " + notes
}
