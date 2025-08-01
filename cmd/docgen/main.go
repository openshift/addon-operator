package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
)

var (
	links = map[string]string{
		"metav1.ObjectMeta":        "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta",
		"metav1.ListMeta":          "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta",
		"metav1.LabelSelector":     "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#labelselector-v1-meta",
		"v1.ResourceRequirements":  "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#resourcerequirements-v1-core",
		"v1.LocalObjectReference":  "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#localobjectreference-v1-core",
		"v1.SecretKeySelector":     "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#secretkeyselector-v1-core",
		"v1.PersistentVolumeClaim": "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#persistentvolumeclaim-v1-core",
		"v1.EmptyDirVolumeSource":  "https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#emptydirvolumesource-v1-core",
	}

	selfLinks = map[string]string{}
)

func sanitizeSectionLink(link string) string {
	link = strings.ToLower(link)
	link = strings.ReplaceAll(link, " ", "-")
	link = strings.ReplaceAll(link, "/", "")
	link = strings.ReplaceAll(link, ".", "")
	return link
}

func printTOC(types []KubeTypes) {
	for _, t := range types {
		strukt := t[0]
		if strukt.Root && strings.Contains(strukt.Name, "List.") {
			// List objects don't inteterest anyone
			continue
		}
		if !strukt.Root {
			fmt.Print("\t")
		}
		fmt.Printf("* [%s](#%s)\n", strukt.ShortName, sanitizeSectionLink(strukt.Name))
	}
}

// Pair of strings. We keed the name of fields and the doc
type Pair struct {
	Name, ShortName, Doc, Type string
	Mandatory, Root            bool
}

// KubeTypes is an array to represent all available types in a parsed file. [0] is for the type itself
type KubeTypes []Pair

// ParseDocumentationFrom gets all types' documentation and returns them as an
// array. Each type is again represented as an array (we have to use arrays as we
// need to be sure for the order of the fields). This function returns fields and
// struct definitions that have no documentation as {name, ""}.
func ParseDocumentationFrom(src string) []KubeTypes {
	a, version := path.Split(path.Dir(src))
	_, group := path.Split(path.Clean(a))
	group = path.Clean(group)
	group = group + ".managed.openshift.io"
	gv := group + "/" + version
	fmt.Fprintln(os.Stderr, src, gv)

	var docForTypes []KubeTypes

	pkg := astFrom(src)

	for _, kubType := range pkg.Types {
		if structType, ok := kubType.Decl.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType); ok {
			var ks KubeTypes
			ks = append(ks, Pair{
				Name:      kubType.Name + "." + gv,
				ShortName: kubType.Name,
				Doc:       fmtRawObjectDoc(kubType.Doc),
				Type:      "",
				Mandatory: false,
				Root:      strings.Contains(kubType.Doc, "+kubebuilder:object:root=true"),
			})

			for _, field := range structType.Fields.List {
				typeString := fieldType(field.Type, gv)
				fieldMandatory := fieldRequired(field)
				if n := fieldName(field); n != "-" {
					fieldDoc := fmtRawFieldDoc(field.Doc.Text())
					ks = append(ks, Pair{Name: n, Doc: fieldDoc, Type: typeString, Mandatory: fieldMandatory})
				}
			}
			docForTypes = append(docForTypes, ks)
		}
	}

	return docForTypes
}

func astFrom(filePath string) *doc.Package {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	apkg, err := doc.NewFromFiles(fset, []*ast.File{f}, filePath)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return apkg
}

func fmtRawFieldDoc(rawDoc string) string {
	var buffer bytes.Buffer
	delPrevChar := func() {
		if buffer.Len() > 0 {
			buffer.Truncate(buffer.Len() - 1) // Delete the last " " or "\n"
		}
	}

	// Ignore all lines after ---
	rawDoc = strings.Split(rawDoc, "---")[0]

	for _, line := range strings.Split(rawDoc, "\n") {
		line = strings.TrimRight(line, " ")
		leading := strings.TrimLeft(line, " ")
		switch {
		case len(line) == 0: // Keep paragraphs
			delPrevChar()
			buffer.WriteString("\n\n")
		case strings.HasPrefix(leading, "TODO"): // Ignore one line TODOs
		case strings.HasPrefix(leading, "+"): // Ignore instructions to go2idl
		default:
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				delPrevChar()
				line = "\n" + line + "\n" // Replace it with newline. This is useful when we have a line with: "Example:\n\tJSON-someting..."
			} else {
				line += " "
			}
			buffer.WriteString(line)
		}
	}

	postDoc := strings.TrimRight(buffer.String(), "\n")

	return postDoc
}

func fmtRawObjectDoc(rawDoc string) string {
	var buffer bytes.Buffer
	delPrevChar := func() {
		if buffer.Len() > 0 {
			buffer.Truncate(buffer.Len() - 1) // Delete the last " " or "\n"
		}
	}

	// Ignore all lines after ---
	rawDoc = strings.Split(rawDoc, "---")[0]

	for _, line := range strings.Split(rawDoc, "\n") {
		line = strings.TrimRight(line, " ")
		leading := strings.TrimLeft(line, " ")
		switch {
		case len(line) == 0: // Keep paragraphs
			delPrevChar()
			buffer.WriteString("\n\n")
		case strings.HasPrefix(leading, "TODO"): // Ignore one line TODOs
		case strings.HasPrefix(leading, "+"): // Ignore instructions to go2idl
		default:
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				delPrevChar()
				line = "\n" + line + "\n" // Replace it with newline. This is useful when we have a line with: "Example:\n\tJSON-someting..."
			} else {
				line += "\n"
			}
			buffer.WriteString(line)
		}
	}

	postDoc := strings.TrimRight(buffer.String(), "\n")

	return postDoc
}

func toLink(typeName string) string {
	selfLink, hasSelfLink := selfLinks[typeName]
	if hasSelfLink {
		return wrapInLink(typeName, selfLink)
	}

	link, hasLink := links[typeName]
	if hasLink {
		return wrapInLink(typeName, link)
	}

	return typeName
}

func wrapInLink(text, link string) string {
	if strings.HasPrefix(link, "#") {
		link = sanitizeSectionLink(link)
	}

	return fmt.Sprintf("[%s](%s)", text, link)
}

// fieldName returns the name of the field as it should appear in JSON format
// "-" indicates that this field is not part of the JSON representation
func fieldName(field *ast.Field) string {
	jsonTag := ""
	if field.Tag != nil {
		jsonTag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1]).Get("json") // Delete first and last quotation
		if strings.Contains(jsonTag, "inline") {
			return "-"
		}
	}

	jsonTag = strings.Split(jsonTag, ",")[0] // This can return "-"
	if jsonTag == "" {
		if field.Names != nil {
			return field.Names[0].Name
		}
		return field.Type.(*ast.Ident).Name
	}
	return jsonTag
}

// fieldRequired returns whether a field is a required field.
func fieldRequired(field *ast.Field) bool {
	jsonTag := ""
	if field.Tag != nil {
		jsonTag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1]).Get("json") // Delete first and last quotation
		return !strings.Contains(jsonTag, "omitempty")
	}

	return false
}

func fieldType(typ ast.Expr, gv string) string {
	switch astType := typ.(type) {
	case *ast.Ident:
		name := astType.Name
		switch name {
		case "string", "int64", "bool":
			return name
		}

		name = name + "." + gv
		return toLink(name)
	case *ast.StarExpr:
		return "*" + toLink(fieldType(astType.X, gv))
	case *ast.SelectorExpr:
		pkg := astType.X.(*ast.Ident)
		t := astType.Sel
		return toLink(pkg.Name + "." + t.Name)
	case *ast.ArrayType:
		return "[]" + toLink(fieldType(astType.Elt, gv))
	case *ast.MapType:
		return "map[" + toLink(fieldType(astType.Key, gv)) + "]" + toLink(fieldType(astType.Value, gv))
	default:
		return ""
	}
}

func printAPIDocs(paths []string, sectionLink string) {
	for _, path := range paths {
		types := ParseDocumentationFrom(path)
		for _, t := range types {
			strukt := t[0]
			selfLinks[strukt.Name] = "#" + strings.ToLower(strukt.Name)
		}
	}

	var types []KubeTypes
	// we need to parse once more to now add the self links
	for _, path := range paths {
		types = append(types, ParseDocumentationFrom(path)...)
	}

	printTOC(types)

	for _, t := range types {
		strukt := t[0]

		if strukt.Root && strings.Contains(strukt.Name, "List.") {
			// List objects don't inteterest anyone
			continue
		}

		fmt.Printf("\n### %s\n\n%s\n\n", strukt.Name, strukt.Doc)

		fmt.Println("| Field | Description | Scheme | Required |")
		fmt.Println("| ----- | ----------- | ------ | -------- |")
		fields := t[1:]

		for _, f := range fields {
			fmt.Println("|", f.Name, "|", f.Doc, "|", f.Type, "|", f.Mandatory, "|")
		}
		fmt.Println("")
		fmt.Printf("[Back to Group](%s)\n", sectionLink)
	}
}

func main() {
	sectionLink := flag.String("section-link", "", "Link to get back to the current section")
	flag.Parse()

	args := flag.Args()
	// args := os.Args[1:]
	if args[0] == "--" {
		args = args[1:]
	}
	sort.Strings(args)
	_, _ = fmt.Fprint(os.Stderr, len(args), args)
	printAPIDocs(args, *sectionLink)
}
