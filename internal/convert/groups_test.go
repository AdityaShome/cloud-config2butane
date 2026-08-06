package convert

import (
	"reflect"
	"testing"

	"github.com/AdityaShome/cloud-config2butane/internal/cloudconfig"
)

func TestGroupsListOfStringsForm(t *testing.T) {
	groups := cloudconfig.GroupList{
		{Name: "docker"},
		{Name: "cloud-users"},
	}
	out, membership := Groups(groups)
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
	out, membership := Groups(groups)
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
	_, membership := Groups(groups)
	want := []string{"admingroup", "docker"}
	if !reflect.DeepEqual(membership["alice"], want) {
		t.Errorf("got %v, want %v", membership["alice"], want)
	}
}

func TestGroupsEmpty(t *testing.T) {
	out, membership := Groups(nil)
	if len(out) != 0 || len(membership) != 0 {
		t.Errorf("got out=%v membership=%v, want both empty", out, membership)
	}
}
