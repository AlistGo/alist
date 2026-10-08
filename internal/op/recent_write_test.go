package op

import (
	"testing"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/model"
)

func TestRecentWriteIdentityKeyBindsToParentID(t *testing.T) {
	storage := &model.Storage{ID: 1, MountPath: "/quark"}
	const name = "x"

	original, ok := recentWriteIdentityKey(storage, "parent-old", name)
	if !ok {
		t.Fatal("original recent-write key rejected")
	}
	afterParentRenameOrMove, ok := recentWriteIdentityKey(storage, "parent-old", name)
	if !ok {
		t.Fatal("same parent identity rejected")
	}
	if original != afterParentRenameOrMove {
		t.Fatalf("same parent ID produced different keys: %q != %q", original, afterParentRenameOrMove)
	}

	recreatedSameNameParent, ok := recentWriteIdentityKey(storage, "parent-new", name)
	if !ok {
		t.Fatal("recreated parent identity rejected")
	}
	if original == recreatedSameNameParent {
		t.Fatal("different parent IDs must never share a recent-write key")
	}

	otherStorage, ok := recentWriteIdentityKey(&model.Storage{ID: 2, MountPath: "/other"}, "parent-old", name)
	if !ok {
		t.Fatal("other storage identity rejected")
	}
	if original == otherStorage {
		t.Fatal("different storage identities must never share a recent-write key")
	}

	reusedMount, ok := recentWriteIdentityKey(&model.Storage{ID: 2, MountPath: "/quark"}, "parent-old", name)
	if !ok {
		t.Fatal("reused mount identity rejected")
	}
	if original == reusedMount {
		t.Fatal("a newly-created storage reusing the same mount path must not inherit recent-write entries")
	}
}

func TestRecentWriteCacheDoesNotRebindChildToRecreatedParent(t *testing.T) {
	storage := &model.Storage{ID: 1, MountPath: "/quark"}
	oldKey, ok := recentWriteIdentityKey(storage, "parent-old", "x")
	if !ok {
		t.Fatal("old key rejected")
	}
	newKey, ok := recentWriteIdentityKey(storage, "parent-new", "x")
	if !ok {
		t.Fatal("new key rejected")
	}

	obj := &model.Object{ID: "fid-x", Name: "x", Size: 1}
	recentWriteCache.Set(oldKey, obj, cache.WithEx[model.Obj](recentWriteConsistencyWindow))
	t.Cleanup(func() {
		recentWriteCache.Del(oldKey)
		recentWriteCache.Del(newKey)
	})

	if _, ok := recentWriteCache.Get(newKey); ok {
		t.Fatal("child written under the old parent rebound to a recreated same-name parent")
	}
	if got, ok := recentWriteCache.Get(oldKey); !ok || got.GetID() != obj.GetID() {
		t.Fatalf("old parent identity lookup = %#v, %v; want fid %q", got, ok, obj.GetID())
	}
}

func TestRecentWriteIdentityKeyRejectsMissingStableIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		storage  *model.Storage
		parentID string
		objName  string
	}{
		{name: "nil storage", storage: nil, parentID: "parent", objName: "x"},
		{name: "missing parent id", storage: &model.Storage{ID: 1, MountPath: "/quark"}, objName: "x"},
		{name: "missing name", storage: &model.Storage{MountPath: "/quark"}, parentID: "parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if key, ok := recentWriteIdentityKey(tc.storage, tc.parentID, tc.objName); ok || key != "" {
				t.Fatalf("key=%q ok=%v, want empty/false", key, ok)
			}
		})
	}
}
