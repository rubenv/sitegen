package sitegen

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/russross/blackfriday"
	"gopkg.in/yaml.v2"
)

func Start() {
	templates = template.Must(template.ParseGlob("templates/*.html"))

	// Crawl the filesystem tree.
	log.Println("==> Crawling")
	content, err := crawlContent()
	if err != nil {
		log.Fatal(err)
	}

	// Wait for parsing
	log.Println("==> Parsing")
	if parseError != nil {
		log.Fatal(parseError)
	}

	// Allow processing metadata
	if processor != nil {
		log.Println("==> Processing")
		content.Process()
		if processError != nil {
			log.Fatal(processError)
		}
	}

	// Generate the output
	log.Println("==> Generating")
	err = os.MkdirAll("static", 0755)
	if err != nil {
		log.Fatal(err)
	}

	content.Write("static")
	if generateError != nil {
		log.Fatal(generateError)
	}
}

var (
	parseError    error = nil
	processError  error = nil
	generateError error = nil
	templates     *template.Template

	processor MetadataProcessor
)

type ContentItem struct {
	Filename string
	FullPath string
	Url      string
	Type     ContentType
	Content  template.HTML
	Children []*ContentItem
	Metadata Metadata
	Extra    interface{}
}

type Metadata struct {
	Title    string
	Template string
	Date     time.Time
}

type metadataTime struct {
	Title    string
	Template string
	Date     string
}

type ContentType int

const (
	Content ContentType = iota
	Directory
	Asset
)

func crawlContent() (*ContentItem, error) {
	return readDir(".", "content")
}

func readDir(name, path string) (*ContentItem, error) {
	fullPath := path + "/" + name
	files, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	c := &ContentItem{
		Filename: name,
		FullPath: fullPath,
		Type:     Directory,
		Children: make([]*ContentItem, 0),
	}

	for _, v := range files {
		var child *ContentItem

		filename := v.Name()
		if isContentFile(filename) {
			parts := strings.Split(filename, ".")
			outname := strings.Join(parts[0:len(parts)-1], ".") + ".html"
			child = &ContentItem{
				Filename: outname,
				FullPath: fullPath + "/" + filename,
				Type:     Content,
			}
			child.Parse(fullPath + "/" + filename)
		} else if v.IsDir() {
			child, err = readDir(filename, fullPath)
			if err != nil {
				return nil, err
			}
		} else {
			child = &ContentItem{
				Filename: filename,
				FullPath: fullPath + "/" + filename,
				Type:     Asset,
			}
		}
		c.Children = append(c.Children, child)
	}

	return c, nil
}

func isContentFile(filename string) bool {
	return strings.HasSuffix(filename, ".html") || strings.HasSuffix(filename, ".md")
}

func splitContent(content []byte) (frontMatter, body []byte, err error) {
	startDelim := []byte("---\n")
	endDelim := []byte("\n---\n\n")
	if bytes.HasPrefix(content, startDelim) {
		endIndex := bytes.Index(content, endDelim)
		if endIndex == -1 {
			err = errors.New("No end delimiter found for metadata!")
			return
		}

		frontMatter = content[len(startDelim):endIndex]
		body = content[endIndex+len(endDelim) : len(content)]
	} else {
		frontMatter = nil
		body = content
	}
	return
}

func (c *ContentItem) parseContent(filename string) error {
	printName := strings.TrimPrefix(filename, "content/.")
	log.Printf(" -> %s\n", printName)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	frontMatter, body, err := splitContent(data)
	if err != nil {
		return err
	}

	if frontMatter != nil {
		yaml.Unmarshal(frontMatter, &c.Metadata)
	}

	if c.Metadata.Template == "" {
		c.Metadata.Template = "page"
	}

	var content []byte
	if strings.HasSuffix(filename, ".md") {
		content = RenderMarkdown(body)
	} else {
		content = body
	}
	c.Content = template.HTML(content)
	return nil
}

func RenderMarkdown(input []byte) []byte {
	// set up the HTML renderer
	htmlFlags := 0
	htmlFlags |= blackfriday.HTML_USE_XHTML
	htmlFlags |= blackfriday.HTML_USE_SMARTYPANTS
	htmlFlags |= blackfriday.HTML_SMARTYPANTS_FRACTIONS
	htmlFlags |= blackfriday.HTML_SMARTYPANTS_LATEX_DASHES
	htmlFlags |= blackfriday.HTML_FOOTNOTE_RETURN_LINKS
	renderer := blackfriday.HtmlRendererWithParameters(htmlFlags, "", "", blackfriday.HtmlRendererParameters{
		FootnoteReturnLinkContents: "↩",
	})

	// set up the parser
	extensions := 0
	extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
	extensions |= blackfriday.EXTENSION_TABLES
	extensions |= blackfriday.EXTENSION_FENCED_CODE
	extensions |= blackfriday.EXTENSION_AUTOLINK
	extensions |= blackfriday.EXTENSION_STRIKETHROUGH
	extensions |= blackfriday.EXTENSION_SPACE_HEADERS
	extensions |= blackfriday.EXTENSION_HEADER_IDS
	extensions |= blackfriday.EXTENSION_FOOTNOTES

	return blackfriday.Markdown(input, renderer, extensions)
}

func (c *ContentItem) Parse(filename string) {
	err := c.parseContent(filename)
	if err != nil {
		parseError = err
	}
}

func (c *ContentItem) Process() {
	c.Url = strings.TrimPrefix(c.FullPath, "content/.")
	extra, err := processor(c)
	if err != nil {
		processError = err
		return
	}
	c.Extra = extra

	for _, v := range c.Children {
		v.Process()
	}
}

func (c *ContentItem) Write(path string) {
	fullPath := path + "/" + c.Filename
	printName := strings.TrimPrefix(fullPath, "static/.")
	if printName != "" {
		log.Printf(" -> %s\n", printName)
	}

	if c.Type == Directory {
		err := os.MkdirAll(fullPath, 0755)
		if err != nil {
			generateError = err
			return
		}
	} else if c.Type == Content {
		err := c.WriteContent(fullPath)
		if err != nil {
			generateError = err
			return
		}
	} else if c.Type == Asset {
		out := strings.Replace(c.FullPath, "content/.", "static", 1)
		err := copyFile(c.FullPath, out)
		if err != nil {
			generateError = err
			return
		}
	}

	for _, v := range c.Children {
		v.Write(fullPath)
	}
}

func (c *ContentItem) WriteContent(path string) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	return templates.ExecuteTemplate(out, c.Metadata.Template, c)
}

// Metadata processing
type MetadataProcessor func(item *ContentItem) (interface{}, error)

func SetMetadataProcessor(f MetadataProcessor) {
	processor = f
}

// Time handling
func (m *Metadata) UnmarshalYAML(unmarshal func(interface{}) error) error {
	md := &metadataTime{}
	if err := unmarshal(md); err != nil {
		return err
	}

	loc, _ := time.LoadLocation("Europe/Brussels")
	t, err := time.ParseInLocation("2006-01-02 15:04:05", md.Date, loc)
	if err != nil {
		return err
	}

	m.Title = md.Title
	m.Template = md.Template
	m.Date = t
	return nil
}

// Utilities

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// copyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
func copyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return fmt.Errorf("copyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("copyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	if err = os.Link(src, dst); err == nil {
		return
	}
	return copyFileContents(src, dst)
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}
