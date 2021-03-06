package views

import (
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

import (
	"github.com/julienschmidt/httprouter"
	"github.com/timtadh/data-structures/errors"
)

import (
	"github.com/timtadh/dynagrok/cmd"
	"github.com/timtadh/dynagrok/localize/discflo"
	"github.com/timtadh/dynagrok/localize/discflo/web/models"
	"github.com/timtadh/dynagrok/localize/discflo/web/models/mem"
)

type Views struct {
	config       *cmd.Config
	opts         *discflo.Options
	assets       string
	tmpl         *template.Template
	sessions     models.SessionStore
	localization *models.Localization
}

func Routes(c *cmd.Config, o *discflo.Options, assetPath string) (http.Handler, error) {
	mux := httprouter.New()
	v := &Views{
		config:   c,
		opts:     o,
		assets:   filepath.Clean(assetPath),
		sessions: mem.NewSessionMapStore("session"),
	}
	mux.GET("/", v.Context(v.Index))
	mux.GET("/blocks", v.Context(v.Blocks))
	mux.GET("/block/:color", v.Context(v.Block))
	mux.GET("/test/:tid/:cid/:nid", v.Context(v.GenerateTest))
	mux.GET("/exclude/:cid", v.Context(v.ExcludeCluster))
	mux.GET("/graph/:cid/:nid/image.png", v.Context(v.Img))
	mux.GET("/graph/:cid/:nid/image.dot", v.Context(v.Dotty))
	mux.GET("/graph/:cid/:nid", v.Context(v.Graph))
	mux.ServeFiles("/static/*filepath", http.Dir(filepath.Join(assetPath, "static")))
	err := v.Init()
	if err != nil {
		return nil, err
	}
	return mux, nil
}

func (v *Views) Init() error {
	err := v.loadTemplates()
	if err != nil {
		return err
	}
	v.localization = models.Localize(v.opts)
	return nil
}

func (v *Views) loadTemplates() error {
	s, err := os.Stat(v.assets)
	if os.IsNotExist(err) {
		return errors.Errorf("Could not load assets from %v. Path does not exist.", v.assets)
	} else if err != nil {
		return err
	}
	v.tmpl = template.New("!")
	if s.IsDir() {
		return v.loadTemplatesFromDir("", filepath.Join(v.assets, "templates"), v.tmpl)
	} else {
		return errors.Errorf("Could not load assets from %v. Unknown file type", v.assets)
	}
}

func (v *Views) loadTemplatesFromDir(ctx, path string, t *template.Template) error {
	dir, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	for _, info := range dir {
		c := filepath.Join(ctx, info.Name())
		p := filepath.Join(path, info.Name())
		if info.IsDir() {
			err := v.loadTemplatesFromDir(c, p, t)
			if err != nil {
				return err
			}
		} else {
			err := v.loadTemplateFile(ctx, p, t)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Views) loadTemplateFile(ctx, path string, t *template.Template) error {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") {
		return nil
	}
	ext := filepath.Ext(name)
	if ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return v.loadTemplate(filepath.Join(ctx, name), string(content), t)
}

func (v *Views) loadTemplate(name, content string, t *template.Template) error {
	log.Println("loaded template", name)
	_, err := t.New(name).Parse(content)
	if err != nil {
		return err
	}
	return nil
}
