package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// The tui's create form: `onctl create -n name --image … --type …`, as
// three fields in the box. Each is typed, and left/right also walks the
// image or type through what `onctl images`/`onctl sizes` list, when the
// provider lists them; an empty field leaves that flag off, so create's
// own default (onctl.yaml's) applies. Enter runs `onctl create` in the
// foreground with those flags.

// tuiCatalogMsg is the images and sizes the form offers, once listed.
// Either may be nil: a provider that lists neither still gets the form,
// with the fields typed.
type tuiCatalogMsg struct {
	images []cloud.CloudImage
	sizes  []cloud.CloudSize
}

type tuiCreateForm struct {
	name, image, typ textinput.Model
	images           []cloud.CloudImage
	sizes            []cloud.CloudSize
	field            int // 0 name, 1 image, 2 type
	loaded           bool
}

const (
	createFieldName = iota
	createFieldImage
	createFieldType
	createFields
)

func newTUICreateForm(image, typ string) *tuiCreateForm {
	in := func(placeholder, value string) textinput.Model {
		t := textinput.New()
		t.Prompt = ""
		t.Placeholder = placeholder
		t.CharLimit = 80
		t.SetValue(value)
		return t
	}
	return &tuiCreateForm{
		name:  in("a generated name", ""),
		image: in("the provider's default", image),
		typ:   in("the provider's default", typ),
	}
}

// openCreate shows the form and lists what it can offer. image and
// typ, when given, are filled in.
func (m tuiModel) openCreate(image, typ string) (tea.Model, tea.Cmd) {
	m.create = newTUICreateForm(image, typ)
	p := m.p
	list := func() tea.Msg {
		var msg tuiCatalogMsg
		msg.images, _ = tuiListImages(p)
		msg.sizes, _ = tuiListSizes(p)
		return msg
	}
	return m, tea.Batch(list, m.create.name.Focus())
}

// setCatalog fills the choices in.
func (f *tuiCreateForm) setCatalog(msg tuiCatalogMsg) {
	f.loaded = true
	f.images, f.sizes = msg.images, msg.sizes
}

func (f *tuiCreateForm) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	*f.active(), cmd = f.active().Update(msg)
	return cmd
}

// active is the field with the cursor.
func (f *tuiCreateForm) active() *textinput.Model {
	switch f.field {
	case createFieldImage:
		return &f.image
	case createFieldType:
		return &f.typ
	}
	return &f.name
}

// cycle walks the active field by n through its catalog, wrapping
// around; from a value not in the catalog (or empty), right goes to the
// first entry and left to the last.
func (f *tuiCreateForm) cycle(n int) {
	var names []string
	var in *textinput.Model
	switch f.field {
	case createFieldImage:
		for _, img := range f.images {
			names = append(names, img.Name)
		}
		in = &f.image
	case createFieldType:
		for _, s := range f.sizes {
			names = append(names, s.Name)
		}
		in = &f.typ
	default:
		return
	}
	if len(names) == 0 {
		return
	}
	i := -1
	for j, name := range names {
		if name == in.Value() {
			i = j
		}
	}
	if i == -1 && n < 0 {
		i = 0
	}
	in.SetValue(names[((i+n)%len(names)+len(names))%len(names)])
	in.CursorEnd()
}

func (m tuiModel) handleCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.create
	switch msg.Type {
	case tea.KeyEsc:
		m.create = nil
		return m, nil
	case tea.KeyTab, tea.KeyDown:
		f.active().Blur()
		f.field = (f.field + 1) % createFields
		return m, f.active().Focus()
	case tea.KeyShiftTab, tea.KeyUp:
		f.active().Blur()
		f.field = (f.field + createFields - 1) % createFields
		return m, f.active().Focus()
	case tea.KeyLeft, tea.KeyRight:
		if f.field != createFieldName {
			n := 1
			if msg.Type == tea.KeyLeft {
				n = -1
			}
			f.cycle(n)
			return m, nil
		}
	case tea.KeyEnter:
		return m.submitCreate()
	}
	var cmd tea.Cmd
	*f.active(), cmd = f.active().Update(msg)
	return m, cmd
}

// submitCreate closes the form and runs `onctl create` with its fields
// in the foreground; the VM shows up on the refresh that follows.
func (m tuiModel) submitCreate() (tea.Model, tea.Cmd) {
	f := m.create
	if !f.loaded {
		return m, m.fail("still listing images and sizes…")
	}
	name, image, typ := strings.TrimSpace(f.name.Value()), strings.TrimSpace(f.image.Value()), strings.TrimSpace(f.typ.Value())
	if name != "" && m.vm(name) != nil {
		return m, m.fail("a VM named %s already exists", name)
	}
	m.create = nil
	return m.runChild("create", name, createArgs(name, image, typ))
}

// createArgs is the `onctl create` command line for the form's fields;
// an empty field leaves its flag off.
func createArgs(name, image, typ string) []string {
	args := []string{"create"}
	if name != "" {
		args = append(args, "-n", name)
	}
	if image != "" {
		args = append(args, "--image", image)
	}
	if typ != "" {
		args = append(args, "--type", typ)
	}
	return tuiChildArgs(args...)
}

// lines is the form, drawn inside the box.
func (f *tuiCreateForm) lines(width int) []string {
	mark := func(field int) string {
		if f.field == field {
			return k9s.prompt.Render(" ▸ ")
		}
		return "   "
	}
	label := func(s string) string { return k9s.infoKey.Render(padRight(s, 7)) }
	lines := []string{"", fit(mark(createFieldName)+label("Name")+f.name.View(), width)}
	lines = append(lines,
		fit(mark(createFieldImage)+label("Image")+f.image.View()+"  "+k9s.dim.Render(f.imageHint()), width),
		fit(mark(createFieldType)+label("Type")+f.typ.View()+"  "+k9s.dim.Render(f.typeHint()), width))
	lines = append(lines, "",
		" "+k9s.hotKey.Render("<tab>")+" "+k9s.hotLabel.Render("next field")+"   "+
			k9s.hotKey.Render("<←/→>")+" "+k9s.hotLabel.Render("pick from the list")+"   "+
			k9s.hotKey.Render("<enter>")+" "+k9s.hotLabel.Render("create")+"   "+
			k9s.hotKey.Render("<esc>")+" "+k9s.hotLabel.Render("cancel"),
		"",
		" "+k9s.dim.Render("Runs `onctl create -n … --image … --type …` in the foreground; you're back here when it's done."))
	return lines
}

// imageHint says what the image field's value is, or what's on offer.
func (f *tuiCreateForm) imageHint() string {
	if !f.loaded {
		return "listing…"
	}
	for _, img := range f.images {
		if img.Name == f.image.Value() && img.Description != "" {
			return img.Description
		}
	}
	if len(f.images) > 0 {
		return fmt.Sprintf("%d images listed", len(f.images))
	}
	return "typed as is"
}

// typeHint says what the type field's size is, or what's on offer.
func (f *tuiCreateForm) typeHint() string {
	if !f.loaded {
		return "listing…"
	}
	for _, s := range f.sizes {
		if s.Name != f.typ.Value() {
			continue
		}
		var parts []string
		if s.VCPU > 0 {
			parts = append(parts, strconv.Itoa(s.VCPU)+" vCPU")
		}
		if s.MemoryMiB > 0 {
			parts = append(parts, formatMiB(s.MemoryMiB))
		}
		if s.DiskGiB > 0 {
			parts = append(parts, strconv.Itoa(s.DiskGiB)+" GiB disk")
		}
		if s.HourlyPrice != "" {
			parts = append(parts, s.HourlyPrice+"/h")
		}
		if s.Default {
			parts = append(parts, "(default)")
		}
		return strings.Join(parts, ", ")
	}
	if len(f.sizes) > 0 {
		return fmt.Sprintf("%d sizes listed", len(f.sizes))
	}
	return "typed as is"
}
