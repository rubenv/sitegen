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

	"github.com/russross/blackfriday"
	"gopkg.in/yaml.v1"
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
	generateError error = nil
	templates     *template.Template
)

type ContentItem struct {
	Filename string
	FullPath string
	Type     ContentType
	Content  template.HTML
	Children []*ContentItem
	Metadata Metadata
}

type Metadata struct {
	Title    string
	Template string
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
		c.Metadata.Template = "post"
	}

	var content []byte
	if strings.HasSuffix(filename, ".md") {
		content = blackfriday.MarkdownCommon(body)
	} else {
		content = body
	}
	c.Content = template.HTML(content)
	return nil
}

func (c *ContentItem) Parse(filename string) {
	err := c.parseContent(filename)
	if err != nil {
		parseError = err
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
