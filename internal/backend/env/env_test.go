package env

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tedilabs/tfvault/internal/backend"
)

func TestGetDefaultPrefix(t *testing.T) {
	t.Setenv("TF_TOKEN_app_terraform_io", "tok-native")
	b, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	token, found, err := b.Get("app.terraform.io")
	if err != nil || !found || token != "tok-native" {
		t.Fatalf("token=%q found=%v err=%v", token, found, err)
	}
	if _, found, _ := b.Get("other.example.com"); found {
		t.Error("unexpected token for unset host")
	}
}

func TestGetCustomPrefixAndDashEncoding(t *testing.T) {
	t.Setenv("CUSTOMER_A_TF_TOKEN_my__registry_example_com", "tok-a")
	b, err := New(map[string]string{"prefix": "CUSTOMER_A_TF_TOKEN_"})
	if err != nil {
		t.Fatal(err)
	}
	token, found, err := b.Get("my-registry.example.com")
	if err != nil || !found || token != "tok-a" {
		t.Fatalf("token=%q found=%v err=%v", token, found, err)
	}
}

func TestEmptyValueIsNotFound(t *testing.T) {
	t.Setenv("TF_TOKEN_empty_example_com", "")
	b, _ := New(nil)
	if _, found, _ := b.Get("empty.example.com"); found {
		t.Error("empty value must be treated as not found")
	}
}

func TestReadOnly(t *testing.T) {
	b, _ := New(nil)
	if err := b.Store("h.example.com", "x"); !errors.Is(err, backend.ErrReadOnly) {
		t.Errorf("Store err = %v, want ErrReadOnly", err)
	}
	if err := b.Forget("h.example.com"); !errors.Is(err, backend.ErrReadOnly) {
		t.Errorf("Forget err = %v, want ErrReadOnly", err)
	}
}

func TestUnknownOption(t *testing.T) {
	if _, err := New(map[string]string{"nope": "x"}); err == nil {
		t.Error("unknown option must be rejected")
	}
}

func TestList(t *testing.T) {
	t.Setenv("MYPFX_app_terraform_io", "a")
	t.Setenv("MYPFX_my__reg_example_com", "b")
	t.Setenv("MYPFX_empty_example_com", "")
	b, _ := New(map[string]string{"prefix": "MYPFX_"})
	hosts, err := b.(*Backend).List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.terraform.io", "my-reg.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Errorf("List() = %v, want %v", hosts, want)
	}
}
