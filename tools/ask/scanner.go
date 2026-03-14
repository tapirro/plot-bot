package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// --- File scanning ---

// scanDiskFiles walks all zones on disk, filtering by extension whitelist.
// This is the DEFAULT mode — indexes all knowledge artifacts regardless of git tracking.
// Binary files (images, audio, video, zips) are excluded by extension filter.
func scanDiskFiles() []string {
	var files []string
	for _, zone := range scanZones {
		zonePath := filepath.Join(root, zone)
		info, err := os.Stat(zonePath)
		if err != nil || !info.IsDir() {
			continue
		}
		filepath.Walk(zonePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// Skip junk directories
			if strings.Contains(path, "__pycache__") || strings.Contains(path, ".DS_Store") {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			isScript := (ext == ".py" || ext == ".sh") && strings.Contains(path, "scripts")
			if !scanExtensions[ext] && !isScript {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
			return nil
		})
	}
	sort.Strings(files)
	return files
}

// scanGitFiles returns only git-tracked files (opt-in via --git-only).
func scanGitFiles() []string {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return scanDiskFiles()
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Check zone match
		inZone := false
		for _, z := range scanZones {
			if strings.HasPrefix(line, z+"/") {
				inZone = true
				break
			}
		}
		if !inZone {
			continue
		}
		ext := strings.ToLower(filepath.Ext(line))
		isScript := (ext == ".py" || ext == ".sh") && strings.Contains(line, "scripts")
		if !scanExtensions[ext] && !isScript {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files
}

// scanAllFiles dispatches to disk (default) or git-only based on flag.
var scanGitOnly bool

func scanAllFiles() []string {
	if scanGitOnly {
		return scanGitFiles()
	}
	return scanDiskFiles()
}

// --- Git dates ---

func gitDatesBatch() (updated, created map[string]string) {
	updated = make(map[string]string)
	created = make(map[string]string)

	cmd := exec.Command("git", "log", "--format=COMMIT %aI", "--name-only", "--diff-filter=ACDMR")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}

	var currentDate string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "COMMIT ") {
			d := line[7:]
			if len(d) >= 10 {
				currentDate = d[:10]
			}
		} else if line != "" && currentDate != "" {
			path := strings.TrimSpace(line)
			if _, ok := updated[path]; !ok {
				updated[path] = currentDate
			}
			created[path] = currentDate
		}
	}
	return
}

// --- Index management ---

func buildIndex() *Index {
	updatedDates, createdDates := gitDatesBatch()
	files := scanAllFiles()
	today := time.Now().Format("2006-01-02")

	var artifacts []Artifact
	for _, relPath := range files {
		fpath := filepath.Join(root, relPath)
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue
		}
		content := string(data)

		fm, bodyStart := parseFrontmatter(content)
		infType, infStatus, infTags := inferFromPath(relPath)

		tier := 0
		if fm != nil {
			if fmString(fm, "id") != "" || fmStringList(fm, "tags") != nil {
				tier = 1
			}
			if fmString(fm, "status") != "" || fmString(fm, "source") != "" || fmString(fm, "type") != "" {
				tier = 2
			}
		}

		typ := fmString(fm, "type")
		if typ == "" {
			typ = infType
		}
		status := fmString(fm, "status")
		if status == "" {
			status = infStatus
		}

		fmTags := fmStringList(fm, "tags")
		tagSet := map[string]bool{}
		for _, t := range infTags {
			tagSet[t] = true
		}
		for _, t := range fmTags {
			tagSet[t] = true
		}
		var tags []string
		for t := range tagSet {
			tags = append(tags, t)
		}
		sort.Strings(tags)

		id := fmString(fm, "id")
		if id == "" {
			id = generateID(relPath, typ)
		}

		title := fmString(fm, "title")
		if title == "" {
			title = getTitle(content, bodyStart)
		}
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
		}

		sections := getSections(content, bodyStart)
		if len(sections) > 0 && tier >= 2 {
			tier = 3
		}

		edges := getEdges(content, relPath, bodyStart)
		source := fmString(fm, "source")
		confidence := fmString(fm, "confidence")
		origin := fmString(fm, "origin")
		basis := fmString(fm, "basis")
		domain := fmString(fm, "domain")
		if domain == "" {
			domain = inferDomain(relPath)
		}

		upd := updatedDates[relPath]
		if upd == "" {
			upd = today
		}
		crt := createdDates[relPath]
		if crt == "" {
			crt = upd
		}

		artifacts = append(artifacts, Artifact{
			ID: id, Type: typ, Status: status, Domain: domain, Tags: tags,
			Title: title, Path: relPath, Updated: upd, Created: crt,
			Sections: sections, EdgesOut: edges, Source: source, Tier: tier,
			Confidence: confidence, Origin: origin, Basis: basis,
		})
	}

	return &Index{
		Artifacts: artifacts,
		Meta: IndexMeta{
			Updated:   today,
			Total:     len(artifacts),
			GitOnly:   scanGitOnly,
			Generator: "ask scan",
		},
	}
}

func indexPath() string {
	return filepath.Join(root, "tools", "index.json")
}

func saveIndex(idx *Index) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(), data, 0644)
}

func loadIndex() *Index {
	data, err := os.ReadFile(indexPath())
	if err != nil {
		idx := buildIndex()
		saveIndex(idx)
		return idx
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		idx2 := buildIndex()
		saveIndex(idx2)
		return idx2
	}
	return &idx
}
