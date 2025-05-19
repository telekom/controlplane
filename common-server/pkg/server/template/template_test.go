package template

import "testing"

func TestTemplate(t *testing.T) {

	template := New(map[string]any{
		"key0": "value0",
		"key1": "$<myValue1>",
		"key2": map[string]any{
			"key3": "$<myValue3>",
		},
		"key4": map[string]any{
			"key5": "myValue5",
		},
	})

	foundPlaceholders := template.GetAllPlaceholders()
	if len(foundPlaceholders) != 2 {
		t.Fatalf("expected 2 placeholders, got %v", len(foundPlaceholders))
	}

	lookUp := map[string]any{
		"myValue1": "value1",
		"myValue3": "value3",
	}

	res, err := template.Apply(lookUp)
	if err != nil {
		t.Fatal(err)
	}

	m := res.(map[string]any)

	if m["key0"] != "value0" {
		t.Fatalf("expected value0, got %v", m["key0"])
	}

	if m["key1"] != "value1" {
		t.Fatalf("expected value1, got %v", m["key1"])
	}

	if m["key2"].(map[string]any)["key3"] != "value3" {
		t.Fatalf("expected value3, got %v", m["key2"].(map[string]any)["key3"])
	}

	if m["key4"].(map[string]any)["key5"] != "myValue5" {
		t.Fatalf("expected myValue5, got %v", m["key4"].(map[string]any)["key5"])
	}

	template = New("value0")
	res, err = template.Apply(lookUp)
	if err != nil {
		t.Fatal(err)
	}

	if res != "value0" {
		t.Fatalf("expected value0, got %v", res)
	}

	template = New("$<myValue1>")
	res, err = template.Apply(lookUp)
	if err != nil {
		t.Fatal(err)
	}

	if res != "value1" {
		t.Fatalf("expected value1, got %v", res)
	}

	template = New("$<myValue1>")
	_, err = template.Apply(map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}

}
