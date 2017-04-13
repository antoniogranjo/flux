package kubernetes

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"text/template"
)

func TestApplyAnnotation(t *testing.T) {
	for _, c := range []struct {
		name         string
		in, ann, out map[string]string
	}{
		{
			name: "has other annotations",
			in:   otherAnnotations,
			ann:  annotation,
			out:  allAnnotations,
		},
		{
			name: "already has annotation",
			in:   annotation,
			ann:  annotation,
			out:  annotation,
		},
		{
			name: "already has annotation and others",
			in:   allAnnotations,
			ann:  annotation,
			out:  allAnnotations,
		},
		{
			name: "no existing annotations",
			in:   noAnnotations,
			ann:  annotation,
			out:  annotation,
		},
		{
			name: "replaces differing annotation",
			in:   allAnnotations,
			ann:  annotation2,
			out:  allAnnotations2,
		},
	} {
		caseIn := templToString(t, annotationsTemplate, c.in)
		fmt.Printf("%s IN:\n%s\n\n", c.name, caseIn)
		caseOut := templToString(t, annotationsTemplate, c.out)
		var trace, out bytes.Buffer
		if err := applyAnnotation(caseIn, c.ann, &trace, &out); err != nil {
			fmt.Fprintln(os.Stderr, "Failed:", c.name)
			fmt.Fprintf(os.Stderr, "--- TRACE ---\n"+trace.String()+"\n---\n")
			t.Fatal(err)
		}
		if string(out.Bytes()) != caseOut {
			fmt.Fprintln(os.Stderr, "Failed:", c.name)
			fmt.Fprintf(os.Stderr, "--- TRACE ---\n"+trace.String()+"\n---\n")
			t.Fatalf("Did not get expected result:\n\n%s\n\nInstead got:\n\n%s", caseOut, string(out.Bytes()))
		}
	}
}

func TestRemoveAnnotation(t *testing.T) {
	for _, c := range []struct {
		name string
		in   map[string]string
		ann  string
		out  map[string]string
	}{
		{
			name: "has other annotations",
			in:   allAnnotations,
			ann:  annotationKey,
			out:  otherAnnotations,
		},
		{
			name: "already has annotation",
			in:   annotation,
			ann:  annotationKey,
			out:  noAnnotations,
		},
		{
			name: "already has annotation and others",
			in:   allAnnotations,
			ann:  annotationKey,
			out:  otherAnnotations,
		},
		{
			name: "no existing annotations",
			in:   noAnnotations,
			ann:  annotationKey,
			out:  noAnnotations,
		},
	} {
		caseIn := templToString(t, annotationsTemplate, c.in)
		caseOut := templToString(t, annotationsTemplate, c.out)
		var trace, out bytes.Buffer
		if err := removeAnnotation(caseIn, c.ann, &trace, &out); err != nil {
			fmt.Fprintln(os.Stderr, "Failed:", c.name)
			fmt.Fprintf(os.Stderr, "--- TRACE ---\n"+trace.String()+"\n---\n")
			t.Fatal(err)
		}
		if string(out.Bytes()) != caseOut {
			fmt.Fprintln(os.Stderr, "Failed:", c.name)
			fmt.Fprintf(os.Stderr, "--- TRACE ---\n"+trace.String()+"\n---\n")
			t.Fatalf("Did not get expected result:\n\n%s\n\nInstead got:\n\n%s", caseOut, string(out.Bytes()))
		}
	}
}

func TestOurAnnotationsOnThePodAreAnError(t *testing.T) {
	t.Error("")
}

var (
	annotationKey    = "flux.policy.automated"
	noAnnotations    = map[string]string(nil)
	annotation       = map[string]string{annotationKey: "true"}
	annotation2      = map[string]string{annotationKey: "false"}
	otherAnnotations = map[string]string{"prometheus.io.scrape": "false"}
	allAnnotations   = map[string]string{annotationKey: "true", "prometheus.io.scrape": "false"}
	allAnnotations2  = map[string]string{annotationKey: "false", "prometheus.io.scrape": "false"}

	annotationsTemplate = template.Must(template.New("").Parse(`---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: nginx{{with .Annotations}}
	annotations:{{range $k, $v := .}}
		{{$k}}: {{printf "%q" $v}}{{end}}{{end}}
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: nginx
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80
`))
)

func templToString(t *testing.T, templ *template.Template, data interface{}) string {
	out := &bytes.Buffer{}
	err := templ.Execute(out, data)
	if err != nil {
		t.Fatal(err)
	}
	return out.String()
}
