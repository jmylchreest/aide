"""Retain blinded inputs and hash final artifacts without duplicating the corpus."""
from pathlib import Path
import hashlib
import json
import os
import shutil

BASE = Path(__file__).resolve().parent
BLIND = Path("/tmp/aide-v6-blind-review")
CORPUS = BASE.parents[1] / "outcomes-v4"


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def package():
    mapping = json.loads((BASE / "blinding.json").read_text())["mapping"]
    reverse = {label:tid for tid,label in mapping.items()}
    entries = []
    for path in sorted(BLIND.rglob("*")):
        if not path.is_file() or "__pycache__" in path.parts:
            continue
        rel = path.relative_to(BLIND)
        if rel.parts[0] not in ("source","tasks","candidates","grading-clarifications.md"):
            continue
        copied = rel.parts[0] != "source" and not (rel.parts[0] == "candidates" and "source" in rel.parts)
        if copied:
            target = BASE / "blind-inputs" / rel
            target.parent.mkdir(parents=True,exist_ok=True)
            shutil.copyfile(path,target)
        elif rel.parts[0] == "source":
            target = CORPUS / "common/template" / Path(*rel.parts[1:])
        else:
            tid = reverse[rel.parts[1]]
            source_rel = Path(*rel.parts[3:])
            target = BASE / "changes" / tid / source_rel
            if not target.exists():
                target = CORPUS / "implement/participant" / source_rel
            if not target.exists():
                target = CORPUS / "common/template" / source_rel
        if sha(path) != sha(target):
            raise ValueError(f"Reviewed input cannot be reconstructed: {rel}")
        entries.append({"path":rel.as_posix(),"bytes":path.stat().st_size,"sha256":sha(path),
                        "retained_copy":target.relative_to(BASE).as_posix() if copied else None,
                        "retained_source":os.path.relpath(target, BASE)})
    (BASE / "blind-inputs-manifest.json").write_text(json.dumps({
        "basis":"Exact reviewed input hashes; source bytes reconstruct from frozen v4 common template, implementation participant harness and preserved changes/tNN. Reconstructed hashes verified during packaging.",
        "files":entries},indent=2)+"\n")
    files = [{"path":p.relative_to(BASE).as_posix(),"bytes":p.stat().st_size,"sha256":sha(p)}
             for p in sorted(BASE.rglob("*")) if p.is_file() and "__pycache__" not in p.parts
             and p.name != "evidence-manifest.json"]
    (BASE / "evidence-manifest.json").write_text(json.dumps({"schema_version":1,
        "basis":"Final result artifact hashes, excluding this manifest and disposable bytecode. Frozen protocol and inherited corpus have separate manifests. Raw provider logs remain locally retained with snapshot hashes in provenance and overhead records.",
        "files":files},indent=2)+"\n")
    print(json.dumps({"reviewed_inputs":len(entries),"result_files":len(files)}))


if __name__ == "__main__":
    package()
