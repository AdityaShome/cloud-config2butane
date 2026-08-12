package convert

import (
	"reflect"
	"testing"

	butane "github.com/coreos/butane/base/v0_5"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func TestGroupsListOfStringsForm(t *testing.T) {
	groups := cloudconfig.GroupList{
		{Name: "docker"},
		{Name: "cloud-users"},
	}
	out, membership, errs := Groups(groups)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []string{"docker", "cloud-users"}
	got := make([]string, len(out))
	for i, g := range out {
		got[i] = g.Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(membership) != 0 {
		t.Errorf("expected no membership entries, got %v", membership)
	}
}

func TestGroupsListOfMapsForm(t *testing.T) {
	groups := cloudconfig.GroupList{
		{Name: "admingroup", Members: []string{"root", "sys"}},
		{Name: "docker"},
	}
	out, membership, errs := Groups(groups)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(out) != 2 || out[0].Name != "admingroup" || out[1].Name != "docker" {
		t.Fatalf("got %+v", out)
	}
	want := map[string][]string{
		"root": {"admingroup"},
		"sys":  {"admingroup"},
	}
	if !reflect.DeepEqual(membership, want) {
		t.Errorf("got membership %v, want %v", membership, want)
	}
}

func TestGroupsMemberOfMultipleGroups(t *testing.T) {
	groups := cloudconfig.GroupList{
		{Name: "admingroup", Members: []string{"alice"}},
		{Name: "docker", Members: []string{"alice"}},
	}
	_, membership, errs := Groups(groups)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []string{"admingroup", "docker"}
	if !reflect.DeepEqual(membership["alice"], want) {
		t.Errorf("got %v, want %v", membership["alice"], want)
	}
}

func TestGroupsEmpty(t *testing.T) {
	out, membership, errs := Groups(nil)
	if len(out) != 0 || len(membership) != 0 || len(errs) != 0 {
		t.Errorf("got out=%v membership=%v errs=%v, want all empty", out, membership, errs)
	}
}

func TestGroupsDuplicateNameRejected(t *testing.T) {
	groups := cloudconfig.GroupList{
		{Name: "wheel"},
		{Name: "wheel", Members: []string{"alice"}},
	}
	out, _, errs := Groups(groups)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if len(out) != 1 || out[0].Name != "wheel" {
		t.Errorf("expected the first occurrence to be kept, got %+v", out)
	}
}

func TestApplyGroupMembershipSkipsAlreadyPresentGroup(t *testing.T) {
	// alice is both an explicit member of a top-level group and lists
	// that same group under her own users[].groups - neither looks
	// redundant alone, but the merge shouldn't produce a duplicate.
	users := []butane.PasswdUser{{Name: "alice", Groups: []butane.Group{"wheel"}}}
	membership := map[string][]string{"alice": {"wheel"}}
	applyGroupMembership(users, membership)
	if got := users[0].Groups; !reflect.DeepEqual(got, []butane.Group{"wheel"}) {
		t.Errorf("got %v, want a single wheel entry, not a duplicate", got)
	}
}
