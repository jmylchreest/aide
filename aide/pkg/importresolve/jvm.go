package importresolve

import (
	"path"
	"sort"
	"strings"
)

var jvmSourceExts = []string{".java", ".kt", ".kts", ".scala", ".sc"}

// jvmResolver maps JVM import statements (Java, Kotlin, Scala — one shared
// package model) to source files: dots become directories under a source
// root, and trailing segments that are classes or members are dropped until
// a file matches (`import a.b.Outer.Inner` lands on a/b/Outer.java). All JVM
// extensions are probed regardless of the importing language — Kotlin
// imports Java classes and vice versa. A wildcard import (`a.b.*`) resolves
// to the package directory. JDK and third-party packages match no source
// root and resolve to "".
type jvmResolver struct {
	fs    *projectFS
	roots []string // project-relative source roots, most specific first
}

func newJVMResolver(pfs *projectFS) *jvmResolver {
	return &jvmResolver{fs: pfs}
}

func (j *jvmResolver) languages() []string { return []string{"java", "kotlin", "scala"} }

func (j *jvmResolver) manifests() []string {
	return []string{"pom.xml", "build.gradle", "build.gradle.kts", "build.sbt"}
}

// addManifest registers the conventional source roots under a build file's
// directory (Maven/Gradle/sbt layout).
func (j *jvmResolver) addManifest(relDir, _ string) {
	for _, set := range []string{"main", "test"} {
		for _, lang := range []string{"java", "kotlin", "scala"} {
			j.addRoot(path.Join(relDir, "src", set, lang))
		}
	}
	j.addRoot(path.Join(relDir, "src")) // plain src/ layouts without the Maven tree
}

func (j *jvmResolver) finalize() {
	sort.Slice(j.roots, func(i, k int) bool {
		if len(j.roots[i]) != len(j.roots[k]) {
			return len(j.roots[i]) > len(j.roots[k])
		}
		return j.roots[i] < j.roots[k]
	})
}

func (j *jvmResolver) resolve(_ string, imp string) string {
	imp = strings.Trim(imp, ".")
	if imp == "" {
		return ""
	}
	segs := strings.Split(imp, ".")
	for _, root := range j.roots {
		// Drop trailing class/member segments until a source file matches.
		for k := len(segs); k >= 1; k-- {
			cand := path.Join(root, strings.Join(segs[:k], "/"))
			for _, ext := range jvmSourceExts {
				if f := cand + ext; j.fs.fileExists(f) {
					return f
				}
			}
		}
		// Wildcard import: the whole path is a package directory.
		pkgDir := path.Join(root, strings.Join(segs, "/"))
		if j.dirHasJVMFiles(pkgDir) {
			return pkgDir
		}
	}
	return ""
}

func (j *jvmResolver) unitOf(file string) string { return file }

// resolveFiles returns the target file, or a wildcard import's package
// members.
func (j *jvmResolver) resolveFiles(fromFile, imp string) []string {
	target := j.resolve(fromFile, imp)
	if target == "" {
		return nil
	}
	for _, ext := range jvmSourceExts {
		if strings.HasSuffix(target, ext) {
			return []string{target}
		}
	}
	var files []string
	for _, name := range j.fs.listFiles(target) {
		for _, ext := range jvmSourceExts {
			if strings.HasSuffix(name, ext) {
				files = append(files, path.Join(target, name))
				break
			}
		}
	}
	return files
}

func (j *jvmResolver) addRoot(rel string) {
	if !j.fs.dirExists(rel) {
		return
	}
	for _, existing := range j.roots {
		if existing == rel {
			return
		}
	}
	j.roots = append(j.roots, rel)
}

func (j *jvmResolver) dirHasJVMFiles(rel string) bool {
	for _, name := range j.fs.listFiles(rel) {
		for _, ext := range jvmSourceExts {
			if strings.HasSuffix(name, ext) {
				return true
			}
		}
	}
	return false
}
