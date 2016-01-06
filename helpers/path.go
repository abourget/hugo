// Copyright 2015 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// filepathPathBridge is a bridge for common functionality in filepath vs path
type filepathPathBridge interface {
	Base(in string) string
	Clean(in string) string
	Dir(in string) string
	Ext(in string) string
	Join(elem ...string) string
	Separator() string
}

type filepathBridge struct {
}

func (filepathBridge) Base(in string) string {
	return filepath.Base(in)
}

func (filepathBridge) Clean(in string) string {
	return filepath.Clean(in)
}

func (filepathBridge) Dir(in string) string {
	return filepath.Dir(in)
}

func (filepathBridge) Ext(in string) string {
	return filepath.Ext(in)
}

func (filepathBridge) Join(elem ...string) string {
	return filepath.Join(elem...)
}

func (filepathBridge) Separator() string {
	return FilePathSeparator
}

var fpb filepathBridge
var sanitizeRegexp = regexp.MustCompile("[^a-zA-Z0-9./_-]")

// MakePath takes a string with any characters and replace it
// so the string could be used in a path.
// It does so by creating a Unicode-sanitized string, with the spaces replaced,
// whilst preserving the original casing of the string.
// E.g. Social Media -> Social-Media
func MakePath(s string) string {
	return UnicodeSanitize(strings.Replace(strings.TrimSpace(s), " ", "-", -1))
}

// MakePathSanitized creates a Unicode-sanitized string, with the spaces replaced
func MakePathSanitized(s string) string {
	if viper.GetBool("DisablePathToLower") {
		return MakePath(s)
	} else {
		return strings.ToLower(MakePath(s))
	}
}

func MakeTitle(inpath string) string {
	return strings.Replace(strings.TrimSpace(inpath), "-", " ", -1)
}

func UnicodeSanitize(s string) string {
	source := []rune(s)
	target := make([]rune, 0, len(source))

	for _, r := range source {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '%' || r == '.' || r == '/' || r == '\\' || r == '_' || r == '-' || r == '#' {
			target = append(target, r)
		}
	}

	var result string

	if viper.GetBool("RemovePathAccents") {
		// remove accents - see https://blog.golang.org/normalization
		t := transform.Chain(norm.NFD, transform.RemoveFunc(isMn), norm.NFC)
		result, _, _ = transform.String(t, string(target))
	} else {
		result = string(target)
	}

	return result
}

func isMn(r rune) bool {
	return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
}

// ReplaceExtension takes a path and an extension, strips the old extension
// and returns the path with the new extension.
func ReplaceExtension(path string, newExt string) string {
	f, _ := fileAndExt(path, fpb)
	return f + "." + newExt
}

func AbsPathify(inPath string) string {
	if filepath.IsAbs(inPath) {
		return filepath.Clean(inPath)
	}

	// TODO(bep): Consider moving workingDir to argument list
	return filepath.Clean(filepath.Join(viper.GetString("WorkingDir"), inPath))
}

func GetStaticDirPath() string {
	return AbsPathify(viper.GetString("StaticDir"))
}

// Get the root directory of the current theme, if there is one.
// If there is no theme, returns the empty string.
func GetThemeDir() string {
	if ThemeSet() {
		return AbsPathify(filepath.Join(viper.GetString("themesDir"), viper.GetString("theme")))
	}
	return ""
}

// GetThemeStaticDirPath returns the theme's static dir path if theme is set.
// If theme is set and the static dir doesn't exist, an error is returned.
func GetThemeStaticDirPath() (string, error) {
	return getThemeDirPath("static")
}

// GetThemeDataDirPath returns the theme's data dir path if theme is set.
// If theme is set and the data dir doesn't exist, an error is returned.
func GetThemeDataDirPath() (string, error) {
	return getThemeDirPath("data")
}

// GetThemeI18nDirPath returns the theme's i18n dir path if theme is set.
// If theme is set and the i18n dir doesn't exist, an error is returned.
func GetThemeI18nDirPath() (string, error) {
	return getThemeDirPath("i18n")
}

func getThemeDirPath(path string) (string, error) {
	var themeDir string
	if ThemeSet() {
		themeDir = filepath.Join(GetThemeDir(), path)
		if _, err := os.Stat(themeDir); os.IsNotExist(err) {
			return "", fmt.Errorf("Unable to find %s directory for theme %s in %s", path, viper.GetString("theme"), themeDir)
		}
	}
	return themeDir, nil
}

// Get the 'static' directory of the current theme, if there is one.
// Ignores underlying errors. Candidate for deprecation?
func GetThemesDirPath() string {
	dir, _ := getThemeDirPath("static")
	return dir
}

func MakeStaticPathRelative(inPath string) (string, error) {
	staticDir := GetStaticDirPath()
	themeStaticDir := GetThemesDirPath()

	return MakePathRelative(inPath, staticDir, themeStaticDir)
}

func MakePathRelative(inPath string, possibleDirectories ...string) (string, error) {

	for _, currentPath := range possibleDirectories {
		if strings.HasPrefix(inPath, currentPath) {
			return strings.TrimPrefix(inPath, currentPath), nil
		}
	}
	return inPath, errors.New("Can't extract relative path, unknown prefix")
}

// Should be good enough for Hugo.
var isFileRe = regexp.MustCompile(".*\\..{1,6}$")

// Expects a relative path starting after the content directory.
func GetDottedRelativePath(inPath string) string {
	inPath = filepath.Clean(filepath.FromSlash(inPath))
	if inPath == "." {
		return "./"
	}
	isFile := isFileRe.MatchString(inPath)
	if !isFile {
		if !strings.HasSuffix(inPath, FilePathSeparator) {
			inPath += FilePathSeparator
		}
	}
	if !strings.HasPrefix(inPath, FilePathSeparator) {
		inPath = FilePathSeparator + inPath
	}
	dir, _ := filepath.Split(inPath)

	sectionCount := strings.Count(dir, FilePathSeparator)

	if sectionCount == 0 || dir == FilePathSeparator {
		return "./"
	}

	var dottedPath string

	for i := 1; i < sectionCount; i++ {
		dottedPath += "../"
	}

	return dottedPath
}

// Filename takes a path, strips out the extension,
// and returns the name of the file.
func Filename(in string) (name string) {
	name, _ = fileAndExt(in, fpb)
	return
}

// FileAndExt returns the filename and any extension of a file path as
// two separate strings.
//
// If the path, in, contains a directory name ending in a slash,
// then both name and ext will be empty strings.
//
// If the path, in, is either the current directory, the parent
// directory or the root directory, or an empty string,
// then both name and ext will be empty strings.
//
// If the path, in, represents the path of a file without an extension,
// then name will be the name of the file and ext will be an empty string.
//
// If the path, in, represents a filename with an extension,
// then name will be the filename minus any extension - including the dot
// and ext will contain the extension - minus the dot.
func fileAndExt(in string, b filepathPathBridge) (name string, ext string) {
	ext = b.Ext(in)
	base := b.Base(in)

	return extractFilename(in, ext, base, b.Separator()), ext
}

func extractFilename(in, ext, base, pathSeparator string) (name string) {

	// No file name cases. These are defined as:
	// 1. any "in" path that ends in a pathSeparator
	// 2. any "base" consisting of just an pathSeparator
	// 3. any "base" consisting of just an empty string
	// 4. any "base" consisting of just the current directory i.e. "."
	// 5. any "base" consisting of just the parent directory i.e. ".."
	if (strings.LastIndex(in, pathSeparator) == len(in)-1) || base == "" || base == "." || base == ".." || base == pathSeparator {
		name = "" // there is NO filename
	} else if ext != "" { // there was an Extension
		// return the filename minus the extension (and the ".")
		name = base[:strings.LastIndex(base, ".")]
	} else {
		// no extension case so just return base, which willi
		// be the filename
		name = base
	}
	return

}

// GetRelativePath returns the relative path of a given path.
func GetRelativePath(path, base string) (final string, err error) {
	if filepath.IsAbs(path) && base == "" {
		return "", errors.New("source: missing base directory")
	}
	name := filepath.Clean(path)
	base = filepath.Clean(base)

	name, err = filepath.Rel(base, name)
	if err != nil {
		return "", err
	}

	if strings.HasSuffix(filepath.FromSlash(path), FilePathSeparator) && !strings.HasSuffix(name, FilePathSeparator) {
		name += FilePathSeparator
	}
	return name, nil
}

func PaginateAliasPath(base string, page int) string {
	paginatePath := viper.GetString("paginatePath")
	uglify := viper.GetBool("UglyURLs")
	var p string
	if base != "" {
		p = filepath.FromSlash(fmt.Sprintf("/%s/%s/%d", base, paginatePath, page))
	} else {
		p = filepath.FromSlash(fmt.Sprintf("/%s/%d", paginatePath, page))
	}
	if uglify {
		p += ".html"
	}

	return p
}

// GuessSection returns the section given a source path.
// A section is the part between the root slash and the second slash
// or before the first slash.
func GuessSection(in string) string {
	parts := strings.Split(in, FilePathSeparator)
	// This will include an empty entry before and after paths with leading and trailing slashes
	// eg... /sect/one/ -> ["", "sect", "one", ""]

	// Needs to have at least a value and a slash
	if len(parts) < 2 {
		return ""
	}

	// If it doesn't have a leading slash and value and file or trailing slash, then return ""
	if parts[0] == "" && len(parts) < 3 {
		return ""
	}

	// strip leading slash
	if parts[0] == "" {
		parts = parts[1:]
	}

	// if first directory is "content", return second directory
	if parts[0] == "content" {
		if len(parts) > 2 {
			return parts[1]
		}
		return ""
	}

	return parts[0]
}

func PathPrep(ugly bool, in string) string {
	if ugly {
		return Uglify(in)
	}
	return PrettifyPath(in)
}

// Same as PrettifyURLPath() but for file paths.
//     /section/name.html       becomes /section/name/index.html
//     /section/name/           becomes /section/name/index.html
//     /section/name/index.html becomes /section/name/index.html
func PrettifyPath(in string) string {
	return prettifyPath(in, fpb)
}

func prettifyPath(in string, b filepathPathBridge) string {
	if filepath.Ext(in) == "" {
		// /section/name/  -> /section/name/index.html
		if len(in) < 2 {
			return b.Separator()
		}
		return b.Join(b.Clean(in), "index.html")
	}
	name, ext := fileAndExt(in, b)
	if name == "index" {
		// /section/name/index.html -> /section/name/index.html
		return b.Clean(in)
	}
	// /section/name.html -> /section/name/index.html
	return b.Join(b.Dir(in), name, "index"+ext)
}

// Extract the root paths from the supplied list of paths.
// The resulting root path will not contain any file separators, but there
// may be duplicates.
// So "/content/section/" becomes "content"
func ExtractRootPaths(paths []string) []string {
	r := make([]string, len(paths))
	for i, p := range paths {
		root := filepath.ToSlash(p)
		sections := strings.Split(root, "/")
		for _, section := range sections {
			if section != "" {
				root = section
				break
			}
		}
		r[i] = root
	}
	return r

}

// FindCWD returns the current working directory from where the Hugo
// executable is run.
func FindCWD() (string, error) {
	serverFile, err := filepath.Abs(os.Args[0])

	if err != nil {
		return "", fmt.Errorf("Can't get absolute path for executable: %v", err)
	}

	path := filepath.Dir(serverFile)
	realFile, err := filepath.EvalSymlinks(serverFile)

	if err != nil {
		if _, err = os.Stat(serverFile + ".exe"); err == nil {
			realFile = filepath.Clean(serverFile + ".exe")
		}
	}

	if err == nil && realFile != serverFile {
		path = filepath.Dir(realFile)
	}

	return path, nil
}

// Same as WriteToDisk but checks to see if file/directory already exists.
func SafeWriteToDisk(inpath string, r io.Reader, fs afero.Fs) (err error) {
	return afero.SafeWriteReader(fs, inpath, r)
}

// Writes content to disk.
func WriteToDisk(inpath string, r io.Reader, fs afero.Fs) (err error) {
	return afero.WriteReader(fs, inpath, r)
}

func GetTempDir(subPath string, fs afero.Fs) string {
	return afero.GetTempDir(fs, subPath)
}

// DirExists checks if a path exists and is a directory.
func DirExists(path string, fs afero.Fs) (bool, error) {
	return afero.DirExists(fs, path)
}

// IsDir checks if a given path is a directory.
func IsDir(path string, fs afero.Fs) (bool, error) {
	return afero.IsDir(fs, path)
}

// IsEmpty checks if a given path is empty.
func IsEmpty(path string, fs afero.Fs) (bool, error) {
	return afero.IsEmpty(fs, path)
}

// Check if a file contains a specified string.
func FileContains(filename string, subslice []byte, fs afero.Fs) (bool, error) {
	return afero.FileContainsBytes(fs, filename, subslice)
}

// Check if a file or directory exists.
func Exists(path string, fs afero.Fs) (bool, error) {
	return afero.Exists(fs, path)
}
