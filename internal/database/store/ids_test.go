package store

import "testing"

func TestDeterministicID_StableAndDistinct(t *testing.T) {
	if a, b := DeterministicID("patient_allergies", "P1", "A1"), DeterministicID("patient_allergies", "P1", "A1"); a != b {
		t.Fatalf("same key must be stable: %s != %s", a, b)
	}
	if DeterministicID("patient_allergies", "P1", "A1") == DeterministicID("patient_allergies", "P1", "A2") {
		t.Error("different key must differ")
	}
	if DeterministicID("patient_allergies", "X", "Y") == DeterministicID("patient_medicines", "X", "Y") {
		t.Error("tables must not collide")
	}
}

func TestBalanceID_KeyedByEntity(t *testing.T) {
	if balanceID("patient", "P1", "USD") != balanceID("patient", "P1", "USD") {
		t.Error("balanceID must be stable")
	}
	if balanceID("patient", "P1", "USD") == balanceID("supplier", "P1", "USD") {
		t.Error("entity_type must affect balanceID")
	}
}
