package legacyimport

import (
	"sort"
	"strconv"
	"strings"

	"clinic-api/internal/database/store"
	"clinic-api/internal/legacyimport/conv"
	"clinic-api/internal/legacyimport/csvutil"
)

func itoa(n int) string { return strconv.Itoa(n) }

func migrateInvoices(c *Context) (t *rowset, numToID map[string]string, err error) {
	f, err := c.File("invoices.csv")
	if err != nil {
		return nil, nil, err
	}
	sec := c.Report.Section("invoices")
	t = newRowset(
		"id", "invoice_number", "from_balance_id", "to_balance_id",
		"amount", "discount_value", "final_amount", "currency_id",
		"notes", "created_by", "created_at", "voided_at")

	col := func(n string) int { return f.Col(n) }
	numToID = map[string]string{}
	var read, badNum, badAmount, discounts, negatives, backfilled int
	numSeen := map[string]bool{}

	for _, row := range f.Rows {
		num := csvutil.Get(row, col("invoice"))
		if num == "" || num == "0" {
			continue
		}
		if numSeen[num] {
			sec.Issue(c.Report, "duplicate invoice number %q dropped.", num)
			continue
		}
		numSeen[num] = true
		read++

		invNum, ok := conv.Int(num, 0)
		if !ok {
			badNum++
		}

		amount, okA := conv.Float(csvutil.Get(row, col("amount")), 0)
		discount, okD := conv.Float(csvutil.Get(row, col("discount")), 0)
		final, okF := conv.Float(csvutil.Get(row, col("final")), 0)
		if !okA || !okD || !okF {
			badAmount++
			sec.Issue(c.Report, "invoice %q: non-numeric amount/discount/final — bad field(s) treated as 0.", num)
		}
		amount, discount, final = store.Round2(amount), store.Round2(discount), store.Round2(final)
		if discount != 0 {
			discounts++
		}
		if amount < 0 || final < 0 {
			negatives++
		}
		currency := normalizeCurrency(csvutil.Get(row, col("currency")))

		created := c.MigrationTS
		if ca, cok, _ := conv.DateTimeZ(csvutil.Get(row, col("createdat"))); cok {
			created = ca
		}

		id := c.ID(created, "invoice:"+num)
		numToID[num] = id

		patientID := csvutil.Get(row, col("patient_id"))
		toBalance := ""
		if patientID != "" && patientID != "0" {
			if !c.knownPatient[patientID] {
				backfilled++
			}
			c.RegisterPatientRef(patientID, csvutil.Get(row, col("name")))
			c.MarkFinancialPatient(patientID)
			toBalance = c.patientBalanceID(patientID)
		}

		t.add(
			id, invNum,
			c.selfBalanceID,
			toBalance,
			amount, discount, final, currency,
			csvutil.Get(row, col("notes")),
			"", // created_by
			created,
			"", // voided_at empty = active
		)
	}

	sec.Counts(read, t.len(), 0)
	sec.Note("Ids are deterministic uuids; invoice_number kept from the CSV.")
	sec.Note("Linked to patients via balances: from_balance = seeded clinic 'self' balance, to_balance = patient balance.")
	if backfilled > 0 {
		sec.Note("%d invoice(s) referenced a patient not in patients.csv (backfilled by name).", backfilled)
	}
	if discounts > 0 {
		sec.Note("%d invoice(s) carry a non-zero discount.", discounts)
	}
	if negatives > 0 {
		sec.Issue(c.Report, "%d invoice(s) have a negative amount (kept as-is; likely refunds/credits).", negatives)
	}
	if badNum > 0 {
		sec.Issue(c.Report, "%d invoice(s) had a non-numeric invoice number (defaulted to 0).", badNum)
	}
	if badAmount > 0 {
		sec.Issue(c.Report, "%d invoice(s) had a non-numeric money field (defaulted to 0).", badAmount)
	}
	return t, numToID, nil
}

func itemType(sheet string) string {
	switch {
	case strings.EqualFold(sheet, "STOCK"):
		return "product"
	case sheet == "":
		return "other"
	default:
		return "procedure"
	}
}

func migrateInvoiceItems(c *Context, numToID map[string]string) (*rowset, error) {
	f, err := c.File("invoice_items.csv")
	if err != nil {
		return nil, err
	}
	sec := c.Report.Section("invoice_items")
	t := newRowset(
		"id", "invoice_id", "item_type", "item_id", "quantity",
		"amount", "final_amount", "notes", "created_at")

	col := func(n string) int { return f.Col(n) }
	var read, orphans, badAmount, procLinked, procUnlinked int
	orphanNums := map[string]bool{}
	lineNo := map[string]int{}

	for _, row := range f.Rows {
		num := csvutil.Get(row, col("invoice"))
		if num == "" || num == "0" {
			continue
		}
		read++
		invID, ok := numToID[num]
		if !ok {
			orphans++
			orphanNums[num] = true
			continue
		}
		lineNo[num]++

		qty, _ := conv.Int(csvutil.Get(row, col("quantity")), 1)
		amount, okA := conv.Float(csvutil.Get(row, col("amount")), 0)
		if !okA {
			badAmount++
		}
		amount = store.Round2(amount)

		name := csvutil.Get(row, col("name"))
		notes := name
		if extra := csvutil.Get(row, col("notes")); extra != "" && extra != name {
			notes = strings.TrimSpace(name + " | " + extra)
		}

		iType := itemType(csvutil.Get(row, col("sheet")))
		itemID := ""
		if iType == "procedure" {
			if itemID = c.resolveProcedureID(name); itemID == "" {
				itemID = c.resolveCategoryProcedureID(csvutil.Get(row, col("procedure")))
			}
			if itemID != "" {
				procLinked++
			} else {
				procUnlinked++
			}
		}

		created := c.MigrationTS
		if ca, cok, _ := conv.DateTimeZ(csvutil.Get(row, col("createdat"))); cok {
			created = ca
		}

		t.add(
			c.ID(created, "invoice_item:"+num+":"+itoa(lineNo[num])),
			invID,
			iType,
			itemID,
			qty,
			amount,
			amount, // final_amount: no per-line discount data
			notes,
			created,
		)
	}

	sec.Counts(read, t.len(), orphans)
	sec.Note("Item name stored in notes (schema has no item name column); item_type derived from source sheet.")
	if procLinked+procUnlinked > 0 {
		sec.Note("Procedure lines linked to a procedure (by name, else by category): %d of %d (%d left with empty item_id).",
			procLinked, procLinked+procUnlinked, procUnlinked)
	}
	if orphans > 0 {
		list := make([]string, 0, len(orphanNums))
		for n := range orphanNums {
			list = append(list, n)
		}
		sort.Strings(list)
		sec.Issue(c.Report, "%d invoice item(s) reference invoice numbers with no header in invoices.csv (skipped): %v.", orphans, list)
	}
	if badAmount > 0 {
		sec.Issue(c.Report, "%d invoice item(s) had a non-numeric amount (defaulted to 0).", badAmount)
	}
	return t, nil
}

func parseInvoiceRef(desc string) (num, kind string) {
	l := strings.ToLower(desc)
	switch {
	case strings.Contains(l, "payment on invoice"):
		kind = "payment"
	case strings.Contains(l, "inv #"), strings.Contains(l, "inv#"), strings.HasPrefix(l, "inv "):
		kind = "charge"
	default:
		return "", ""
	}
	if i := strings.Index(desc, "#"); i >= 0 {
		j := i + 1
		for j < len(desc) && desc[j] == ' ' {
			j++
		}
		k := j
		for k < len(desc) && desc[k] >= '0' && desc[k] <= '9' {
			k++
		}
		num = desc[j:k]
	}
	return num, kind
}

func migrateTransactions(c *Context, numToID map[string]string) (*rowset, error) {
	f, err := c.File("transactions.csv")
	if err != nil {
		return nil, err
	}
	sec := c.Report.Section("balance_transactions")
	t := newRowset(
		"id", "from_balance_id", "to_balance_id", "amount", "currency_id",
		"transaction_type", "transaction_method", "source_type", "source_id",
		"description", "created_by", "created_at", "voided_at")

	col := func(n string) int { return f.Col(n) }
	var read, backfilled, unknownType, badAmount int
	unknownTypes := map[string]bool{}
	seq := 0

	for _, row := range f.Rows {
		patientID := csvutil.Get(row, col("patient_id"))
		if patientID == "" || patientID == "0" {
			continue
		}
		read++
		if !c.knownPatient[patientID] {
			backfilled++
		}
		c.RegisterPatientRef(patientID, csvutil.Get(row, col("name")))
		c.MarkFinancialPatient(patientID)

		amount, okA := conv.Float(csvutil.Get(row, col("amount")), 0)
		if !okA {
			badAmount++
		}
		amount = store.Round2(amount)
		currency := normalizeCurrency(csvutil.Get(row, col("currency")))
		desc := csvutil.Get(row, col("descriptin"))
		rawType := csvutil.Get(row, col("type"))

		invNum, kind := parseInvoiceRef(desc)
		if kind == "" {
			switch rawType {
			case "RECEIPT", "PAYMENT", "CREDIT NOTE", "Receipt Other Serie":
				kind = "payment"
			case "Invoices":
				kind = "charge"
			default:
				kind = "payment"
				unknownType++
				unknownTypes[rawType] = true
			}
		}

		patBal := c.patientBalanceID(patientID)
		var from, to, txType string
		if kind == "charge" {
			from, to, txType = c.selfBalanceID, patBal, "charge"
		} else {
			from, to, txType = patBal, c.selfBalanceID, "payment"
		}

		sourceType, sourceID := "", ""
		if invNum != "" {
			if id, ok := numToID[invNum]; ok {
				sourceType, sourceID = "invoice", id
			}
		}

		created := c.MigrationTS
		if ca, cok, _ := conv.DateTimeZ(csvutil.Get(row, col("created_at"))); cok {
			created = ca
		}

		seq++
		t.add(
			c.ID(created, "txn:"+itoa(seq)),
			from, to, amount, currency,
			txType,
			"cash", // source method is always blank; schema default is cash
			sourceType, sourceID,
			desc,
			"", // created_by
			created,
			"", // voided_at empty = active
		)
	}

	sec.Counts(read, t.len(), 0)
	sec.Note("type=Invoices split into 'charge' (INV # N) and 'payment' (Payment on Invoice # N); RECEIPT/PAYMENT/CREDIT NOTE -> payment.")
	sec.Note("transaction_method defaulted to 'cash' (blank in source). Invoice-linked txns carry source_type='invoice'.")
	if backfilled > 0 {
		sec.Note("%d transaction(s) referenced a patient not in patients.csv (backfilled by name).", backfilled)
	}
	if unknownType > 0 {
		list := make([]string, 0, len(unknownTypes))
		for s := range unknownTypes {
			list = append(list, s)
		}
		sec.Issue(c.Report, "%d transaction(s) had an unrecognized type (treated as payment): %v.", unknownType, list)
	}
	if badAmount > 0 {
		sec.Issue(c.Report, "%d transaction(s) had a non-numeric amount (defaulted to 0).", badAmount)
	}
	return t, nil
}

func buildPatientBalances(c *Context) *rowset {
	sec := c.Report.Section("balances")
	t := newRowset(
		"id", "entity_type", "entity_id", "entity_name", "currency_id",
		"amount", "total_in", "total_out", "created_at")

	patients := append([]string(nil), c.financialPatients...)
	sort.Strings(patients)
	for _, id := range patients {
		t.add(
			c.patientBalanceID(id),
			"patient", id, c.patientName[id], usdCurrency,
			float64(0), float64(0), float64(0),
			c.MigrationTS,
		)
	}

	sec.Counts(len(patients), t.len(), 0)
	sec.Note("One USD balance per patient with financial activity (deterministic ids matching the app's scheme).")
	sec.Note("amount/total_in/total_out are recomputed from the imported transactions by the app's own balance function; the seeded 'self' balance is recomputed too.")
	return t
}
