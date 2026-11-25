package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Checkpoint is a restorable snapshot of one file taken before an agent edit.
type Checkpoint struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Label   string    `json:"label"`
	Rel     string    `json:"rel"`
	Existed bool      `json:"existed"`
	Mode    uint32    `json:"mode,omitempty"`
}

// CheckpointsDir is the hidden workspace directory that stores checkpoints.
func CheckpointsDir(root string) string {
	return filepath.Join(root, ".monet", "checkpoints")
}

// Snapshot records the current contents of rel (relative to root). A file that
// does not exist yet is recorded too, so rewinding removes a created file.
func Snapshot(root, rel, label string) (Checkpoint, error) {
	abs, err := SafeJoin(root, rel)
	if err != nil {
		return Checkpoint{}, err
	}
	if abs == root {
		return Checkpoint{}, fmt.Errorf("refusing to checkpoint the workspace root")
	}
	if strings.HasPrefix(Rel(root, abs), ".monet/") {
		return Checkpoint{}, fmt.Errorf("refusing to checkpoint Monet's own state")
	}
	now := time.Now().UTC()
	id := now.Format("20060102-150405.000000000")
	id = strings.ReplaceAll(id, ".", "-")
	cp := Checkpoint{ID: id, Time: now, Label: label, Rel: Rel(root, abs)}
	dir := filepath.Join(CheckpointsDir(root), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Checkpoint{}, err
	}

	info, statErr := os.Stat(abs)
	if statErr == nil {
		if !info.Mode().IsRegular() {
			return Checkpoint{}, fmt.Errorf("%s is not a regular file", rel)
		}
		if info.Size() > 4<<20 {
			return Checkpoint{}, fmt.Errorf("%s is larger than 4 MB", rel)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return Checkpoint{}, err
		}
		cp.Existed = true
		cp.Mode = uint32(info.Mode().Perm())
		if err := os.WriteFile(filepath.Join(dir, "content"), data, 0o600); err != nil {
			return Checkpoint{}, err
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return Checkpoint{}, statErr
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return Checkpoint{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), append(data, '\n'), 0o644); err != nil {
		return Checkpoint{}, err
	}
	return cp, nil
}

// ListCheckpoints returns checkpoints newest first.
func ListCheckpoints(root string) ([]Checkpoint, error) {
	entries, err := os.ReadDir(CheckpointsDir(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Checkpoint
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(CheckpointsDir(root), e.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil || cp.Rel == "" {
			continue
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// Rewind restores a checkpoint by id.
func Rewind(root, id string) (Checkpoint, error) {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." {
		return Checkpoint{}, fmt.Errorf("checkpoint id is required")
	}
	dir := filepath.Join(CheckpointsDir(root), id)
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return Checkpoint{}, fmt.Errorf("checkpoint %s not found", id)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, err
	}
	target, err := SafeJoin(root, cp.Rel)
	if err != nil {
		return Checkpoint{}, err
	}
	if cp.Existed {
		content, err := os.ReadFile(filepath.Join(dir, "content"))
		if err != nil {
			return Checkpoint{}, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Checkpoint{}, err
		}
		mode := fs.FileMode(cp.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, content, mode); err != nil {
			return Checkpoint{}, err
		}
	} else if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Checkpoint{}, err
	}
	return cp, nil
}

// RewindLast restores the most recent checkpoint.
func RewindLast(root string) (Checkpoint, error) {
	list, err := ListCheckpoints(root)
	if err != nil {
		return Checkpoint{}, err
	}
	if len(list) == 0 {
		return Checkpoint{}, fmt.Errorf("no checkpoints yet")
	}
	return Rewind(root, list[0].ID)
}
