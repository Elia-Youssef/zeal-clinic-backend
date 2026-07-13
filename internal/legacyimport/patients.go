package legacyimport

import (
	"strings"

	"clinic-api/internal/legacyimport/conv"
	"clinic-api/internal/legacyimport/csvutil"
)

var patientColumns = []string{
	"id", "first_name", "middle_name", "last_name", "gender", "date_of_birth",
	"contact", "email", "emergency_contact_name", "emergency_contact_phone",
	"weight", "height", "blood_type", "address", "referral_source", "notes",
	"created_at",
}

type patientBuilder struct {
	rows  *rowset
	dupes int
}

func loadPatients(c *Context) (*patientBuilder, error) {
	f, err := c.File("patients.csv")
	if err != nil {
		return nil, err
	}
	sec := c.Report.Section("patients")
	t := newRowset(patientColumns...)

	col := func(n string) int { return f.Col(n) }
	var (
		read, dupes          int
		garbage              int
		emptyGender          int
		blankDOB, badDOB     int
		badBlood             int
		badWeight, badHeight int
	)
	idSeen := map[string]bool{}

	for _, row := range f.Rows {
		id := csvutil.Get(row, col("id"))
		if id == "" || id == "0" {
			continue
		}
		if idSeen[id] {
			dupes++
			sec.Issue(c.Report, "duplicate patient id %q — keeping first, dropping duplicate.", id)
			continue
		}

		first := csvutil.Get(row, col("firstnam"))
		middle := csvutil.Get(row, col("midlnam"))
		last := csvutil.Get(row, col("lastnam"))

		if !conv.HasLetter(first) && !conv.HasLetter(middle) && !conv.HasLetter(last) {
			garbage++
			continue
		}
		idSeen[id] = true
		read++
		c.knownPatient[id] = true

		c.patientName[id] = strings.TrimSpace(strings.Join(strings.Fields(first+" "+middle+" "+last), " "))

		gender := conv.NormalizeGender(csvutil.Get(row, col("gender")))
		if gender == "" {
			emptyGender++
		}

		dob := ""
		if rawDOB := csvutil.Get(row, col("datbirth")); conv.IsBlankDate(rawDOB) {
			blankDOB++
		} else if d, ok, derr := conv.Date(rawDOB, true); derr != nil {
			badDOB++
			sec.Issue(c.Report, "patient %q: unparseable date_of_birth %q (left empty).", id, rawDOB)
		} else if ok {
			dob = d
		}

		blood, unrec := conv.NormalizeBloodType(csvutil.Get(row, col("bloodtype")))
		if unrec {
			badBlood++
		}

		weight, okw := conv.Float(csvutil.Get(row, col("weight")), 0)
		if !okw {
			badWeight++
		}
		height, okh := conv.Float(csvutil.Get(row, col("height")), 0)
		if !okh {
			badHeight++
		}

		created := c.MigrationTS
		if ca, ok, _ := conv.DateTimeZ(csvutil.Get(row, col("createdat"))); ok {
			created = ca
		}

		t.add(
			id, first, middle, last, gender, dob,
			csvutil.Get(row, col("phone")),
			csvutil.Get(row, col("email")),
			csvutil.Get(row, col("emcontnam")),
			csvutil.Get(row, col("emcontphn")),
			weight, height, blood,
			csvutil.Get(row, col("address")),
			csvutil.Get(row, col("refersourc")),
			csvutil.Get(row, col("notes")),
			created,
		)
	}

	sec.Note("Gender is empty in the legacy export; written as empty string. %d patients affected.", emptyGender)
	sec.Note("date_of_birth: %d blank placeholders -> empty; %d parsed.", blankDOB, read-blankDOB-badDOB)
	if dupes > 0 {
		sec.Issue(c.Report, "%d duplicate patient id(s) dropped.", dupes)
	}
	if garbage > 0 {
		sec.Issue(c.Report, "%d patient row(s) had no real name (dots/dashes/digits only) and were skipped; referenced ones are backfilled from their reference name.", garbage)
	}
	if badBlood > 0 {
		sec.Issue(c.Report, "%d unrecognized blood_type value(s) blanked (known fixes applied: '0+'->'O+', 'ab+'->'AB+', etc.).", badBlood)
	}
	if badWeight > 0 {
		sec.Issue(c.Report, "%d non-numeric weight value(s) defaulted to 0.", badWeight)
	}
	if badHeight > 0 {
		sec.Issue(c.Report, "%d non-numeric height value(s) defaulted to 0.", badHeight)
	}
	sec.Counts(read, t.len(), dupes)
	return &patientBuilder{rows: t, dupes: dupes}, nil
}

func (pb *patientBuilder) finalize(c *Context) *rowset {
	sec := c.Report.Section("patients")
	realCount := pb.rows.len()

	for _, id := range c.backfillIDs {
		name := conv.ParseFullName(c.backfillName[id])
		pb.rows.add(
			id, name.First, name.Middle, name.Last,
			"", // gender unknown
			"", // date_of_birth
			"", // contact
			"", // email
			"", // emergency_contact_name
			"", // emergency_contact_phone
			float64(0), float64(0),
			"", // blood_type
			"", // address
			"", // referral_source
			"Backfilled during migration from a reference in appointments/invoices/transactions; only the name was available.",
			c.MigrationTS,
		)
	}

	if len(c.backfillIDs) > 0 {
		sec.Note("Backfilled %d patient(s) referenced by appointments/invoices/transactions but missing from patients.csv (name only).", len(c.backfillIDs))
		sec.Issue(c.Report, "%d patient(s) referenced elsewhere were not in patients.csv and were created from their name with empty details.", len(c.backfillIDs))
	}
	sec.Counts(realCount, pb.rows.len(), pb.dupes)
	return pb.rows
}
