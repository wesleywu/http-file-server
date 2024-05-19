package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	tarGzKey         = "tar.gz"
	tarGzValue       = "true"
	tarGzContentType = "application/x-tar+gzip"

	zipKey         = "zip"
	zipValue       = "true"
	zipContentType = "application/zip"

	osPathSeparator = string(filepath.Separator)
)

const directoryListingTemplateText = `
<html>
<head>
	<title>Index of {{ .Title }}</title>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<link rel="stylesheet" href="/static/layout/autoindex.css" type="text/css">
</head>
<body>
<h1>Index of {{ .Title }}</h1>
{{ if or .Files .AllowUpload }}
<table>
	<thead>
		<th class="indexcolicon">
			<img src="/static/icons/blank.png" alt="[ICO]">
		</th>
		<th class="indexcolname">
			<a href="?C=N;O=D">Name</a>
		</th>
		<th class="indexcollastmod">
			<a href="?C=M;O=A">Last modified</a>
		</th>
		<th class="indexcolsize">
			<a href="?C=S;O=A">Size</a>
		</th>   
	</thead>
	<tbody>
	{{- if .ParentDir }}
		<tr class="even">
			<td class="indexcolicon"><a href="/"><img src="/static/icons/go-previous.png" alt="[PARENTDIR]"></a></td>
			<td class="indexcolname"><a href="{{ .ParentDir.String }}">Parent Directory</a></td><td class="indexcollastmod">&nbsp;</td>
			<td class="indexcolsize">  - </td>
		</tr>
	{{- end }}
	{{- range .Files }}
		<tr>
			{{ if (not .IsDir) }}
 				<td class="indexcolicon"><a href="{{ .URL.String }}"><img src="/static/icons/package-x-generic.png" alt="[ARC]"></a></td>
				<td class="indexcolname"><a href="{{ .URL.String }}">{{ .Name }}</a></td>
				<td class="indexcollastmod">{{ .LastModified }}</td>
				<td class="indexcolsize">{{ .Size | printf "%d" }}</td>
			{{ else }}
				<td class="indexcolicon"><a href="{{ .URL.String }}"><img src="/static/icons/folder.png" alt="[DIR]"></a></td>
				<td class="indexcolname"><a href="{{ .URL.String }}">{{ .Name }}</a></td>
				<td class="indexcollastmod">{{ .LastModified }}</td>
				<td class="indexcolsize">  - </td>
			{{ end }}
		</tr>
	{{- end }}
	</tbody>
</table>
{{ end }}
</body>
</html>
`

type fileSizeBytes int64

func (f fileSizeBytes) String() string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	divBy := func(x int64) int {
		return int(math.Round(float64(f) / float64(x)))
	}
	switch {
	case f < KB:
		return fmt.Sprintf("%d", f)
	case f < MB:
		return fmt.Sprintf("%dK", divBy(KB))
	case f < GB:
		return fmt.Sprintf("%dM", divBy(MB))
	case f >= GB:
		fallthrough
	default:
		return fmt.Sprintf("%dG", divBy(GB))
	}
}

type directoryListingFileData struct {
	Name         string
	Size         fileSizeBytes
	IsDir        bool
	URL          *url.URL
	LastModified string
}

type directoryListingData struct {
	Title       string
	ZipURL      *url.URL
	TarGzURL    *url.URL
	Files       []directoryListingFileData
	AllowUpload bool
	ParentDir   *url.URL
}

type fileHandler struct {
	route       string
	path        string
	allowUpload bool
	allowDelete bool
}

var (
	directoryListingTemplate = template.Must(template.New("").Parse(directoryListingTemplateText))
)

func (f *fileHandler) serveStatus(w http.ResponseWriter, r *http.Request, status int) error {
	w.WriteHeader(status)
	_, err := w.Write([]byte(http.StatusText(status)))
	if err != nil {
		return err
	}
	return nil
}

func (f *fileHandler) serveTarGz(w http.ResponseWriter, r *http.Request, path string) error {
	w.Header().Set("Content-Type", tarGzContentType)
	name := filepath.Base(path) + ".tar.gz"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	return tarGz(w, path)
}

func (f *fileHandler) serveZip(w http.ResponseWriter, r *http.Request, osPath string) error {
	w.Header().Set("Content-Type", zipContentType)
	name := filepath.Base(osPath) + ".zip"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	return zip(w, osPath)
}

func (f *fileHandler) serveDir(w http.ResponseWriter, r *http.Request, osPath string) error {
	d, err := os.Open(osPath)
	if err != nil {
		return err
	}
	files, err := d.Readdir(-1)
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return directoryListingTemplate.Execute(w, directoryListingData{
		AllowUpload: f.allowUpload,
		ParentDir: func() *url.URL {
			urlStr := r.URL.String()
			if strings.HasSuffix(urlStr, "/") {
				urlStr = urlStr[0 : len(urlStr)-1]
			}
			lastSlashPos := strings.LastIndex(urlStr, "/")
			if lastSlashPos > 1 {
				parentUrlStr := urlStr[0:lastSlashPos]
				parentUrl, err := url.Parse(parentUrlStr)
				if err != nil {
					return nil
				}
				return parentUrl
			}
			return nil
		}(),
		Title: func() string {
			relPath, _ := filepath.Rel(f.path, osPath)
			return filepath.Join(filepath.Base(f.path), relPath)
		}(),
		TarGzURL: func() *url.URL {
			url := *r.URL
			q := url.Query()
			q.Set(tarGzKey, tarGzValue)
			url.RawQuery = q.Encode()
			return &url
		}(),
		ZipURL: func() *url.URL {
			url := *r.URL
			q := url.Query()
			q.Set(zipKey, zipValue)
			url.RawQuery = q.Encode()
			return &url
		}(),
		Files: func() (out []directoryListingFileData) {
			for _, d := range files {
				name := d.Name()
				if d.IsDir() {
					name += osPathSeparator
				}
				fileData := directoryListingFileData{
					Name:         name,
					IsDir:        d.IsDir(),
					Size:         fileSizeBytes(d.Size()),
					LastModified: d.ModTime().Format("2006-01-02 15:04:05"),
					URL: func() *url.URL {
						url := *r.URL
						url.Path = path.Join(url.Path, name)
						if d.IsDir() {
							url.Path += "/"
						}
						return &url
					}(),
				}
				out = append(out, fileData)
			}
			return out
		}(),
	})
}

func (f *fileHandler) serveUploadTo(w http.ResponseWriter, r *http.Request, osPath string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	in, h, err := r.FormFile("file")
	if err == http.ErrMissingFile {
		w.Header().Set("Location", r.URL.String())
		w.WriteHeader(303)
	}
	if err != nil {
		return err
	}
	outPath := filepath.Join(osPath, filepath.Base(h.Filename))
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	w.Header().Set("Location", r.URL.String())
	w.WriteHeader(303)
	return nil
}

// ServeHTTP is http.Handler.ServeHTTP
func (f *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s %s %s", f.path, r.RemoteAddr, r.Method, r.URL.String())
	urlPath := r.URL.Path
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	urlPath = strings.TrimPrefix(urlPath, f.route)
	urlPath = strings.TrimPrefix(urlPath, "/"+f.route)

	osPath := strings.ReplaceAll(urlPath, "/", osPathSeparator)
	osPath = filepath.Clean(osPath)
	osPath = filepath.Join(f.path, osPath)
	info, err := os.Stat(osPath)
	switch {
	case os.IsNotExist(err):
		_ = f.serveStatus(w, r, http.StatusNotFound)
	case os.IsPermission(err):
		_ = f.serveStatus(w, r, http.StatusForbidden)
	case err != nil:
		_ = f.serveStatus(w, r, http.StatusInternalServerError)
	case !f.allowDelete && r.Method == http.MethodDelete:
		_ = f.serveStatus(w, r, http.StatusForbidden)
	case !f.allowUpload && r.Method == http.MethodPost:
		_ = f.serveStatus(w, r, http.StatusForbidden)
	case r.URL.Query().Get(zipKey) != "":
		err := f.serveZip(w, r, osPath)
		if err != nil {
			_ = f.serveStatus(w, r, http.StatusInternalServerError)
		}
	case r.URL.Query().Get(tarGzKey) != "":
		err := f.serveTarGz(w, r, osPath)
		if err != nil {
			_ = f.serveStatus(w, r, http.StatusInternalServerError)
		}
	case f.allowUpload && info.IsDir() && r.Method == http.MethodPost:
		err := f.serveUploadTo(w, r, osPath)
		if err != nil {
			_ = f.serveStatus(w, r, http.StatusInternalServerError)
		}
	case f.allowDelete && !info.IsDir() && r.Method == http.MethodDelete:
		err := os.Remove(osPath)
		if err != nil {
			_ = f.serveStatus(w, r, http.StatusInternalServerError)
		}
	case info.IsDir():
		err := f.serveDir(w, r, osPath)
		if err != nil {
			_ = f.serveStatus(w, r, http.StatusInternalServerError)
		}
	default:
		http.ServeFile(w, r, osPath)
	}
}
