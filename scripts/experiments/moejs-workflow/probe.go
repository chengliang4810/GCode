package workflowprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/MoeForge/moejs"
	"github.com/MoeForge/moejs/eventloop"
)

type node struct {
	Input          string          `json:"input"`
	Status         string          `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	CommitSequence int             `json:"commitSequence,omitempty"`
}

type journal struct {
	ScriptHash     string          `json:"scriptHash"`
	Nodes          map[string]node `json:"nodes"`
	CommitSequence int             `json:"commitSequence"`
	path           string
	mu             sync.Mutex
}

func openJournal(path, source string) (*journal, error) {
	hash := sha256.Sum256([]byte(source))
	want := hex.EncodeToString(hash[:])
	j := &journal{ScriptHash: want, Nodes: make(map[string]node), path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, j); err != nil {
		return nil, err
	}
	if j.ScriptHash != want {
		return nil, errors.New("script hash changed")
	}
	return j, nil
}

func (j *journal) admit(site, input string) (json.RawMessage, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if n, ok := j.Nodes[site]; ok {
		if n.Input != input {
			return nil, false, fmt.Errorf("input changed at %s", site)
		}
		if n.Status == "completed" {
			return append(json.RawMessage(nil), n.Result...), true, nil
		}
	}
	j.Nodes[site] = node{Input: input, Status: "running"}
	return nil, false, j.save()
}

func (j *journal) complete(site string, result json.RawMessage) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := j.Nodes[site]
	j.CommitSequence++
	n.Status, n.Result, n.CommitSequence = "completed", result, j.CommitSequence
	j.Nodes[site] = n
	return j.save()
}

func (j *journal) save() error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(j.path), ".journal-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(name, j.path)
}

type worker func(context.Context, string, string) (any, error)

// run reconstructs the script from the beginning. It deliberately only caches
// completed results: it does NOT implement the settlement replay scheduler.
func run(ctx context.Context, source, path string, work worker) (json.RawMessage, error) {
	j, err := openJournal(path, source)
	if err != nil {
		return nil, err
	}
	mod, err := moejs.CompileTS("probe.dwf.ts", source)
	if err != nil {
		return nil, err
	}
	hook, err := mod.Hook("run")
	if err != nil {
		return nil, err
	}
	rt := moejs.NewRuntime(moejs.Options{})
	loop, err := eventloop.New(rt, eventloop.Options{})
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	err = rt.SetGlobal("ask", moejs.NativeFunc(func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		site, input := moejs.Arg(args, 0).String(), moejs.Arg(args, 1).String()
		cached, hit, err := j.admit(site, input)
		if err != nil {
			return moejs.Undefined(), err
		}
		if hit {
			value, err := rt.ParseJSON(cached)
			if err != nil {
				return moejs.Undefined(), err
			}
			// Keep ask's Promise contract on both live and cached paths.
			p, resolve, _ := rt.NewPromise()
			if err := resolve(value); err != nil {
				return moejs.Undefined(), err
			}
			return p, nil
		}
		p, settle := loop.NewPromise()
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, workErr := work(runCtx, site, input)
			var data json.RawMessage
			if workErr == nil {
				data, workErr = json.Marshal(result)
			}
			if workErr == nil {
				workErr = j.complete(site, data)
			}
			settle(func(rt *moejs.Runtime) (moejs.Value, error) {
				if workErr != nil {
					return moejs.Undefined(), workErr
				}
				return rt.ParseJSON(data)
			})
		}()
		return p, nil
	}))
	if err != nil {
		return nil, err
	}
	stopped := make(chan struct{})
	stop := context.AfterFunc(runCtx, func() {
		loop.Interrupt(context.Cause(runCtx))
		close(stopped)
	})
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	var result moejs.Value
	err = loop.Run(func(rt *moejs.Runtime) error {
		if err := rt.Load(mod); err != nil {
			return err
		}
		result, err = rt.Call(hook)
		return err
	})
	if err != nil {
		return nil, err
	}
	if state, value, ok := moejs.PromiseResult(result); ok {
		if state != moejs.PromiseFulfilled {
			return nil, fmt.Errorf("script promise state %v: %s", state, value.String())
		}
		result = value
	}
	return rt.AppendJSON(nil, result)
}
