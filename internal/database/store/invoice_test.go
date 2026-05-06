package store

import (
	"database/sql"
	"errors"
	"testing"
)

// makeProductWithStock seeds a product with a starting quantity and unit price.
func makeProductWithStock(t *testing.T, name string, qty int, price float64) Product {
	t.Helper()
	p := Product{Name: name, Quantity: qty, UnitPrice: price}
	if err := p.Create(); err != nil {
		t.Fatalf("Product.Create: %v", err)
	}
	return p
}

func productQuantity(t *testing.T, id string) int {
	t.Helper()
	var q int
	if err := RDB.QueryRow(`SELECT quantity FROM products WHERE id = ?`, id).Scan(&q); err != nil {
		t.Fatalf("productQuantity: %v", err)
	}
	return q
}

func discountRedeemed(t *testing.T, id string) bool {
	t.Helper()
	var redeemedAt sql.NullString
	if err := RDB.QueryRow(`SELECT redeemed_at FROM discounts WHERE id = ?`, id).Scan(&redeemedAt); err != nil {
		t.Fatalf("discountRedeemed: %v", err)
	}
	return redeemedAt.Valid
}

func TestInvoice_IsValid(t *testing.T) {
	t.Run("ok with positive amount", func(t *testing.T) {
		inv := Invoice{Amount: 50}
		if err := inv.IsValid(); err != nil {
			t.Errorf("got %v", err)
		}
	})
	t.Run("zero amount allowed", func(t *testing.T) {
		inv := Invoice{Amount: 0}
		if err := inv.IsValid(); err != nil {
			t.Errorf("got %v", err)
		}
	})
	t.Run("negative amount rejected", func(t *testing.T) {
		inv := Invoice{Amount: -1}
		if err := inv.IsValid(); err == nil {
			t.Errorf("expected error")
		}
	})
}

// Client (patient to self) invoice tests

func TestInvoice_Create_ClientInvoice_BasicTotals(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Pay", "Tient", "1234567")
	pb := patientBalance(t, pat.ID, cur.ID)

	inv := Invoice{
		FromBalanceID: pb.ID,
		ToBalanceID:   self.ID,
		CurrencyID:    cur.ID,
		Items: InvoiceItemList{
			{ItemType: "other", Quantity: 1, Amount: 30},
			{ItemType: "other", Quantity: 1, Amount: 70},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if inv.ID == "" {
		t.Errorf("ID not assigned")
	}
	if !approxEqual(inv.Amount, 100) || !approxEqual(inv.FinalAmount, 100) {
		t.Errorf("inv totals = %v / %v want 100 / 100", inv.Amount, inv.FinalAmount)
	}
	if inv.InvoiceNumber < 1 {
		t.Errorf("InvoiceNumber not set: %d", inv.InvoiceNumber)
	}

	// Charge balance transaction (final amount).
	if n := countRows(t, "balance_transactions", "source_type='invoice' AND source_id=? AND transaction_type='charge'", inv.ID); n != 1 {
		t.Errorf("expected 1 charge tx, got %d", n)
	}

	// Patient balance went DOWN by 100, self UP by 100.
	pAfter := fetchBalance(t, pb.ID)
	sAfter := fetchBalance(t, self.ID)
	if !approxEqual(pAfter.Amount, -100) {
		t.Errorf("patient.Amount = %v want -100", pAfter.Amount)
	}
	if !approxEqual(sAfter.Amount, self.Amount+100) {
		t.Errorf("self.Amount diff = %v want +100", sAfter.Amount-self.Amount)
	}
	// Charge should not affect total_in/total_out.
	if !approxEqual(pAfter.TotalOut, 0) || !approxEqual(sAfter.TotalIn, 0) {
		t.Errorf("flow totals leaked from charge; pAfter=%+v sAfter=%+v", pAfter, sAfter)
	}
}

func TestInvoice_Create_InvoiceNumberSequence_PerEntityType(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	p1 := makePatient(t, "First", "P", "1112221")
	p2 := makePatient(t, "Second", "P", "1112222")
	b1 := patientBalance(t, p1.ID, cur.ID)
	b2 := patientBalance(t, p2.ID, cur.ID)

	inv1 := Invoice{FromBalanceID: b1.ID, ToBalanceID: self.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 10}}}
	if err := inv1.Create(); err != nil {
		t.Fatal(err)
	}
	inv2 := Invoice{FromBalanceID: b2.ID, ToBalanceID: self.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 20}}}
	if err := inv2.Create(); err != nil {
		t.Fatal(err)
	}
	if inv2.InvoiceNumber != inv1.InvoiceNumber+1 {
		t.Errorf("expected sequential invoice numbers, got %d -> %d", inv1.InvoiceNumber, inv2.InvoiceNumber)
	}
}

// Invoice-level percentage offer reduces final_amount; charge tx records the
// post-discount amount and tags transaction_method as "discount".
func TestInvoice_Create_InvoiceDiscount_Percentage(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Disc", "P", "9990001")
	pb := patientBalance(t, pat.ID, cur.ID)

	d := Discount{Name: "10off", DiscountType: "offer", ValueType: "percentage", Value: 10, IsActive: 1}
	if err := d.Create(); err != nil {
		t.Fatal(err)
	}

	inv := Invoice{
		FromBalanceID: pb.ID, ToBalanceID: self.ID, CurrencyID: cur.ID,
		DiscountID: d.ID,
		Items: InvoiceItemList{
			{ItemType: "other", Quantity: 1, Amount: 200},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(inv.Amount, 200) || !approxEqual(inv.DiscountValue, 20) || !approxEqual(inv.FinalAmount, 180) {
		t.Errorf("totals = %v / %v / %v want 200 / 20 / 180", inv.Amount, inv.DiscountValue, inv.FinalAmount)
	}
	// Charge tx should record the FINAL amount (180), not the gross.
	var chargedAmount float64
	if err := RDB.QueryRow(`SELECT amount FROM balance_transactions WHERE source_id = ? AND transaction_type='charge'`, inv.ID).Scan(&chargedAmount); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(chargedAmount, 180) {
		t.Errorf("charge amount = %v want 180", chargedAmount)
	}
	// Charge transaction method recorded as "discount".
	var method string
	RDB.QueryRow(`SELECT transaction_method FROM balance_transactions WHERE source_id = ? AND transaction_type='charge'`, inv.ID).Scan(&method)
	if method != "discount" {
		t.Errorf("charge tx method = %q want discount", method)
	}
}

// Invoice-level fixed offer caps at the invoice gross amount.
func TestInvoice_Create_InvoiceDiscount_FixedCappedAtAmount(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Cap", "P", "9990002")
	pb := patientBalance(t, pat.ID, cur.ID)

	// Fixed $50 off, but the invoice is only $30: discount must be capped at 30.
	d := Discount{Name: "50off-fixed", DiscountType: "offer", ValueType: "fixed", Value: 50, IsActive: 1}
	if err := d.Create(); err != nil {
		t.Fatal(err)
	}

	inv := Invoice{
		FromBalanceID: pb.ID, ToBalanceID: self.ID, CurrencyID: cur.ID,
		DiscountID: d.ID,
		Items: InvoiceItemList{
			{ItemType: "other", Quantity: 1, Amount: 30},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(inv.FinalAmount, 0) {
		t.Errorf("final amount should be 0 (fully discounted), got %v", inv.FinalAmount)
	}
	if !approxEqual(inv.DiscountValue, 30) {
		t.Errorf("discount value should cap at 30, got %v", inv.DiscountValue)
	}
}

// A gift line with a code creates an unredeemed gift discount; no auto-apply
// transaction is recorded (redemption is later, via the code).
func TestInvoice_Create_GiftLineWithCode_CreatesUnredeemedGift(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Gift", "P", "9990003")
	pb := patientBalance(t, pat.ID, cur.ID)

	preDiscounts := countRows(t, "discounts", "")

	code := "GIFT-XYZ"
	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: pb.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{
			{
				ItemType: "gift",
				Quantity: 1,
				Amount:   100,
				GiftCode: &code,
				Notes:    "GiftCard 100",
			},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	// One new discount of type gift.
	if got := countRows(t, "discounts", "discount_type = 'gift' AND code = ?", code); got != 1 {
		t.Errorf("expected gift discount with code %s, got %d", code, got)
	}
	if got := countRows(t, "discounts", ""); got != preDiscounts+1 {
		t.Errorf("expected 1 new discount row, got %d -> %d", preDiscounts, got)
	}
	// Item ItemID rewritten to the new discount.
	var giftDiscountID string
	RDB.QueryRow(`SELECT id FROM discounts WHERE code = ?`, code).Scan(&giftDiscountID)
	if inv.Items[0].ItemID != giftDiscountID {
		t.Errorf("item.ItemID = %q want %q", inv.Items[0].ItemID, giftDiscountID)
	}
	// Not redeemed yet.
	if discountRedeemed(t, giftDiscountID) {
		t.Errorf("gift should not be redeemed at creation when only code is set")
	}
	// No adjustment tx sourced from the invoice (only the charge).
	if n := countRows(t, "balance_transactions", "source_type='invoice' AND source_id=? AND transaction_type='adjustment'", inv.ID); n != 0 {
		t.Errorf("expected 0 adjustment tx, got %d", n)
	}
}

// A gift line assigned to a patient_id auto-applies as credit on that
// patient's balance (FROM=patient, TO=self, type=adjustment).
func TestInvoice_Create_GiftLineWithPatientID_AutoCredits(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	buyer := makePatient(t, "Buyer", "P", "9990011")
	bb := patientBalance(t, buyer.ID, cur.ID)
	recipient := makePatient(t, "Recip", "P", "9990012")
	rb := patientBalance(t, recipient.ID, cur.ID)

	rid := recipient.ID
	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: bb.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{
			{
				ItemType:      "gift",
				Quantity:      1,
				Amount:        80,
				GiftPatientID: &rid,
				Notes:         "Gift for Recip",
			},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}

	// Charge: buyer balance went UP by 80 (owes), self DOWN by 80.
	bAfter := fetchBalance(t, bb.ID)
	if !approxEqual(bAfter.Amount, 80) {
		t.Errorf("buyer.Amount = %v want 80", bAfter.Amount)
	}
	// Apply-gift: recipient balance went DOWN by 80 (credit / negative).
	rAfter := fetchBalance(t, rb.ID)
	if !approxEqual(rAfter.Amount, -80) {
		t.Errorf("recipient.Amount = %v want -80", rAfter.Amount)
	}
	// Apply-gift tx is sourced from the invoice and has type=adjustment.
	if n := countRows(t, "balance_transactions",
		"source_type='invoice' AND source_id=? AND transaction_type='adjustment' AND transaction_method='discount'", inv.ID); n != 1 {
		t.Errorf("expected 1 apply-gift adjustment, got %d", n)
	}
	// Gift is marked redeemed.
	var giftID string
	RDB.QueryRow(`SELECT id FROM discounts WHERE patient_id = ?`, rid).Scan(&giftID)
	if !discountRedeemed(t, giftID) {
		t.Errorf("gift should be redeemed after auto-apply")
	}
}

// Supplier (supplier to self) invoice: product stock should INCREASE on delivery.

func TestInvoice_Create_SupplierInvoice_AddsProductStock(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)

	supID := "sup-1"
	sb := Balance{EntityType: "supplier", EntityID: &supID, EntityName: "Acme", CurrencyID: cur.ID}
	if err := sb.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	prod := makeProductWithStock(t, "Widget", 10, 5)
	preStock := productQuantity(t, prod.ID)

	// supplier (from) to self (to): goods flow into stock.
	inv := Invoice{
		FromBalanceID: sb.ID,
		ToBalanceID:   self.ID,
		CurrencyID:    cur.ID,
		Items: InvoiceItemList{
			{ItemType: "product", ItemID: prod.ID, Quantity: 7, Amount: 35},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if got := productQuantity(t, prod.ID); got != preStock+7 {
		t.Errorf("stock = %d want %d", got, preStock+7)
	}
}

// Client invoice with product item: stock should DECREASE.

func TestInvoice_Create_ClientInvoice_DecrementsProductStock(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Stock", "P", "9990004")
	pb := patientBalance(t, pat.ID, cur.ID)

	prod := makeProductWithStock(t, "Lotion", 5, 12)

	// self (from) to patient (to): goods leave stock.
	inv := Invoice{
		FromBalanceID: self.ID,
		ToBalanceID:   pb.ID,
		CurrencyID:    cur.ID,
		Items: InvoiceItemList{
			{ItemType: "product", ItemID: prod.ID, Quantity: 2, Amount: 24},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if got := productQuantity(t, prod.ID); got != 3 {
		t.Errorf("stock = %d want 3", got)
	}
}

// Invoice.Delete reverses every side effect

func TestInvoice_Delete_ReversesStockAndCharge(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Rev", "P", "9990005")
	pb := patientBalance(t, pat.ID, cur.ID)
	prod := makeProductWithStock(t, "Cream", 8, 20)

	d := Discount{Name: "10pct", DiscountType: "offer", ValueType: "percentage", Value: 10, IsActive: 1}
	if err := d.Create(); err != nil {
		t.Fatal(err)
	}

	// self -> patient with a product item AND an invoice-level offer discount.
	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: pb.ID, CurrencyID: cur.ID,
		DiscountID: d.ID,
		Items: InvoiceItemList{
			{ItemType: "product", ItemID: prod.ID, Quantity: 3, Amount: 60},
			{ItemType: "other", Quantity: 1, Amount: 100},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(inv.FinalAmount, 144) {
		t.Fatalf("final amount = %v want 144 (160 - 16)", inv.FinalAmount)
	}
	stockAfterCreate := productQuantity(t, prod.ID)
	if stockAfterCreate != 5 {
		t.Fatalf("stock after create = %d want 5", stockAfterCreate)
	}

	// Delete: stock should restore, charge tx removed.
	if err := inv.Delete(); err != nil {
		t.Fatal(err)
	}

	if got := productQuantity(t, prod.ID); got != 8 {
		t.Errorf("stock after delete = %d want 8 (restored)", got)
	}
	if n := countRows(t, "balance_transactions", "source_type='invoice' AND source_id=?", inv.ID); n != 0 {
		t.Errorf("charge tx leaked, got %d", n)
	}
	// Patient balance restored to ~0.
	pAfter := fetchBalance(t, pb.ID)
	if !approxEqual(pAfter.Amount, 0) {
		t.Errorf("patient balance after delete = %v want 0", pAfter.Amount)
	}
	if n := countRows(t, "invoice_items", "invoice_id = ?", inv.ID); n != 0 {
		t.Errorf("items not cascaded, got %d", n)
	}
	if n := countRows(t, "invoices", "id = ?", inv.ID); n != 0 {
		t.Errorf("invoice not deleted")
	}
}

// Deleting an invoice that created an unredeemed code-only gift also drops
// the gift discount row.
func TestInvoice_Delete_DropsUnredeemedGift(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	buyer := makePatient(t, "Buy", "P", "9990013")
	bb := patientBalance(t, buyer.ID, cur.ID)

	code := "GIFT-DEL"
	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: bb.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{
			{ItemType: "gift", Quantity: 1, Amount: 25, GiftCode: &code, Notes: "g"},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if err := inv.Delete(); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, "discounts", "code = ?", code); n != 0 {
		t.Errorf("gift discount leaked, got %d", n)
	}
}

// Deleting an invoice whose code-only gift was already redeemed should fail.
func TestInvoice_Delete_RefusesIfGiftRedeemed(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	buyer := makePatient(t, "Buy", "P", "9990014")
	bb := patientBalance(t, buyer.ID, cur.ID)
	recip := makePatient(t, "Rcp", "P", "9990015")
	patientBalance(t, recip.ID, cur.ID)

	code := "GIFT-USED"
	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: bb.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{
			{ItemType: "gift", Quantity: 1, Amount: 40, GiftCode: &code, Notes: "g"},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	// Redeem against the recipient.
	if _, err := ApplyGiftByCode(code, recip.ID, cur.ID, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := inv.Delete(); err == nil {
		t.Errorf("expected delete to fail because gift was redeemed")
	}
}

func TestInvoice_Delete_NotFound(t *testing.T) {
	setupTestDB(t)
	inv := Invoice{ID: "ghost"}
	err := inv.Delete()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestInvoice_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	var inv Invoice
	err := inv.GetByID("ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestInvoice_GetByID_LoadsItems(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Get", "P", "9990006")
	pb := patientBalance(t, pat.ID, cur.ID)

	inv := Invoice{
		FromBalanceID: pb.ID, ToBalanceID: self.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{
			{ItemType: "other", Quantity: 1, Amount: 50, Notes: "consultation"},
			{ItemType: "other", Quantity: 1, Amount: 30, Notes: "refill"},
		},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}

	var got Invoice
	if err := got.GetByID(inv.ID); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(got.Items))
	}
	if got.FromEntityName == "" {
		t.Errorf("expected FromEntityName join, got empty")
	}
}

func TestInvoice_Update_OnlyNotesField(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Upd", "P", "9990007")
	pb := patientBalance(t, pat.ID, cur.ID)

	inv := Invoice{
		FromBalanceID: pb.ID, ToBalanceID: self.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 10}},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}
	if err := inv.Update(map[string]any{"notes": "hello"}); err != nil {
		t.Fatal(err)
	}
	if inv.Notes != "hello" {
		t.Errorf("notes = %q", inv.Notes)
	}
	// Unknown keys ignored, no UpdatedAt change.
	prev := inv.UpdatedAt
	if err := inv.Update(map[string]any{"amount": 999}); err != nil {
		t.Fatal(err)
	}
	if inv.UpdatedAt != prev {
		t.Errorf("update with no recognized keys should not bump UpdatedAt")
	}
	if !approxEqual(inv.Amount, 10) {
		t.Errorf("amount should be unchanged, got %v", inv.Amount)
	}
}

func TestInvoiceList_GetClientInvoices_FiltersByPatient(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	p1 := makePatient(t, "Client1", "P", "9990008")
	p2 := makePatient(t, "Client2", "P", "9990009")
	b1 := patientBalance(t, p1.ID, cur.ID)
	b2 := patientBalance(t, p2.ID, cur.ID)

	for i := 0; i < 2; i++ {
		inv := Invoice{FromBalanceID: self.ID, ToBalanceID: b1.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 10}}}
		if err := inv.Create(); err != nil {
			t.Fatal(err)
		}
	}
	inv := Invoice{FromBalanceID: self.ID, ToBalanceID: b2.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 10}}}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}

	var list InvoiceList
	if err := list.GetClientInvoices(p1.ID); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("p1 invoices = %d want 2", len(list))
	}

	list = nil
	if err := list.GetClientInvoices(""); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("all client invoices = %d want 3", len(list))
	}
}

func TestInvoiceList_GetSupplierInvoices_FiltersBySupplier(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	s1ID := "sup-A"
	s1 := Balance{EntityType: "supplier", EntityID: &s1ID, EntityName: "A", CurrencyID: cur.ID}
	if err := s1.GetOrCreate(); err != nil {
		t.Fatal(err)
	}
	s2ID := "sup-B"
	s2 := Balance{EntityType: "supplier", EntityID: &s2ID, EntityName: "B", CurrencyID: cur.ID}
	if err := s2.GetOrCreate(); err != nil {
		t.Fatal(err)
	}

	inv1 := Invoice{FromBalanceID: s1.ID, ToBalanceID: self.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 10}}}
	if err := inv1.Create(); err != nil {
		t.Fatal(err)
	}
	inv2 := Invoice{FromBalanceID: s2.ID, ToBalanceID: self.ID, CurrencyID: cur.ID, Items: InvoiceItemList{{ItemType: "other", Quantity: 1, Amount: 20}}}
	if err := inv2.Create(); err != nil {
		t.Fatal(err)
	}

	var list InvoiceList
	if err := list.GetSupplierInvoices(s1ID); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != inv1.ID {
		t.Errorf("got %+v", list)
	}
}

func TestInvoiceList_GetByItem(t *testing.T) {
	setupTestDB(t)
	cur := seededCurrency(t)
	self := seededSelfBalance(t, cur.ID)
	pat := makePatient(t, "Item", "P", "9990010")
	pb := patientBalance(t, pat.ID, cur.ID)
	prod := makeProductWithStock(t, "Tonic", 100, 5)

	inv := Invoice{
		FromBalanceID: self.ID, ToBalanceID: pb.ID, CurrencyID: cur.ID,
		Items: InvoiceItemList{{ItemType: "product", ItemID: prod.ID, Quantity: 1, Amount: 5}},
	}
	if err := inv.Create(); err != nil {
		t.Fatal(err)
	}

	var list InvoiceList
	if err := list.GetByItem(prod.ID, "product"); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != inv.ID {
		t.Errorf("got %+v", list)
	}
}
