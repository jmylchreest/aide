import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import unittest


SPEC = importlib.util.spec_from_file_location("isolated_bridge", Path(__file__).with_name("bridge.py"))
bridge = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(bridge)


class BridgeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="aide-bridge-test-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.root = self.base / "root"
        self.root.mkdir()
        (self.root / "source.ts").write_text("export function FixtureOnly() { return 1; }\n")
        self.evidence = self.base / "evidence"
        self.binary = self.base / "fake-aide"
        self.binary.write_text(f"#!{sys.executable}\n" + textwrap.dedent('''\
            import hashlib, json, os, pathlib, sys, time
            root = pathlib.Path(os.environ['AIDE_PROJECT_ROOT'])
            if sys.argv[1:] != ['mcp']:
                print('Indexed fixture')
                sys.exit(0)
            if (root / 'stall-initialize').exists():
                time.sleep(30)
            for raw in sys.stdin:
                request = json.loads(raw)
                if 'id' not in request:
                    continue
                method = request['method']
                if method == 'initialize':
                    result = {'protocolVersion':'2024-11-05','capabilities':{},'serverInfo':{'name':'fake','version':'1'}}
                elif request['params']['name'] == 'instance_info':
                    socket = str(root / '.aide/aide.sock')
                    if len(socket.encode()) > 100:
                        socket = '/tmp/aide/' + hashlib.sha256(str(root).encode()).hexdigest()[:16] + '.sock'
                    identity = {'project_root':str(root),'real_project_root':str(root),'cwd':str(root),
                                'db_path':str(root / '.aide/memory/memory.db'),'socket_path':socket}
                    if (root / 'wrong-identity').exists():
                        identity['db_path'] = '/outside/.aide/memory/memory.db'
                    result = {'content':[{'type':'text','text':json.dumps(identity)}]}
                else:
                    (root / 'handler-called').write_text('yes')
                    result = {'content':[{'type':'text','text':'actual handler text\\n'}],
                              '_meta':{'receipt':'retained'},'observed_args':request['params']['arguments']}
                print(json.dumps({'jsonrpc':'2.0','id':request['id'],'result':result}), flush=True)
            '''))
        self.binary.chmod(0o700)

    def prepare(self):
        return bridge.prepare(self.root, self.binary, self.evidence)

    def call(self, name="code_search", arguments=None):
        return bridge.call(self.root, self.binary, self.evidence,
                           {"name": name, "arguments": arguments or {"query": "FixtureOnly"}})

    def test_setup_refuses_existing_aide_or_symlink_before_indexing(self):
        (self.root / ".aide").mkdir()
        with self.assertRaisesRegex(ValueError, "pristine"):
            self.prepare()
        (self.root / ".aide").rmdir()
        (self.root / "leak.ts").symlink_to(self.binary)
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.prepare()
        self.assertFalse(self.evidence.exists())

    def test_setup_requires_explicit_absolute_and_separate_paths(self):
        for root, binary, evidence in [
            ("relative", self.binary, self.evidence),
            (self.root, self.binary, self.root / "evidence"),
            (self.root, self.binary, self.base),
            (self.root, self.root / "source.ts", self.evidence),
        ]:
            with self.subTest(root=root, evidence=evidence), self.assertRaises(ValueError):
                bridge.prepare(root, binary, evidence)

    def test_unprepared_fixture_and_changed_binary_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "not been prepared"):
            self.call()
        self.prepare()
        self.binary.write_text(self.binary.read_text() + "\n# changed\n")
        with self.assertRaisesRegex(ValueError, "binding"):
            self.call()

    def test_calls_reject_other_evidence_and_runtime_symlink(self):
        self.prepare()
        with self.assertRaisesRegex(ValueError, "binding"):
            bridge.call(self.root, self.binary, self.base / "other", {"name":"code_search","arguments":{"query":"x"}})
        (self.root / ".aide" / "memory").symlink_to(self.base, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "Runtime symlink"):
            self.call()

    def test_boundary_requests_do_not_invoke_handler(self):
        self.prepare()
        bad = [
            {"name":"memory_search","arguments":{"query":"x"}},
            {"name":"code_symbols","arguments":{"file":"../secret.ts"}},
            {"name":"code_outline","arguments":{"file":str(self.binary)}},
            {"name":"code_read_symbol","arguments":{"symbol":"FixtureOnly","file":".aide/retrieval-bridge.json"}},
            {"name":"code_search","arguments":{"query":"x","file":"../"}},
            {"name":"code_search","arguments":{"query":"x","root":"/outside"}},
            {"name":"code_search","arguments":{"query":"x","limit":True}},
            {"name":"code_read_symbol","arguments":{"symbols":["x"] * 11}},
        ]
        for request in bad:
            with self.subTest(request=request), self.assertRaises(ValueError):
                bridge.call(self.root, self.binary, self.evidence, request)
        self.assertFalse((self.root / "handler-called").exists())
        failures = [json.loads(p.read_text()) for p in self.evidence.glob("*.json") if p.name != "setup.json"]
        self.assertEqual(len(failures), len(bad))
        self.assertTrue(all(row["error"] and not row["transcript"] for row in failures))

    def test_symlink_added_after_setup_blocks_even_index_search(self):
        self.prepare()
        (self.root / "escape.ts").symlink_to(self.binary)
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.call()
        self.assertFalse((self.root / "handler-called").exists())

    def test_filters_remain_substrings_but_source_paths_must_exist(self):
        request = bridge.validate_request(self.root, {"name":"code_search","arguments":{"query":"x","file":"src/auth"}})
        self.assertEqual(request["arguments"]["file"], "src/auth")
        request = bridge.validate_request(self.root, {"name":"code_outline","arguments":{"file":str(self.root / "source.ts")}})
        self.assertEqual(request["arguments"]["file"], "source.ts")
        with self.assertRaisesRegex(ValueError, "existing"):
            bridge.validate_request(self.root, {"name":"code_symbols","arguments":{"file":"missing.ts"}})

    def test_environment_has_no_live_defaults_or_home(self):
        env = bridge.child_environment(self.root)
        self.assertNotIn("HOME", env)
        self.assertEqual(env["AIDE_PROJECT_ROOT"], str(self.root))
        self.assertEqual(env["AIDE_CODE_WATCH"], "false")
        self.assertEqual(env["AIDE_INDEX_NON_VCS"], "false")
        self.assertEqual(env["AIDE_GRAMMAR_AUTO_DOWNLOAD"], "0")

    def test_identity_checked_before_requested_handler(self):
        (self.root / "wrong-identity").touch()
        self.prepare()
        with self.assertRaisesRegex(RuntimeError, "identity"):
            self.call()
        self.assertFalse((self.root / "handler-called").exists())

    def test_actual_text_only_and_full_protocol_timing_evidence(self):
        self.prepare()
        command = [sys.executable, str(Path(bridge.__file__)), "call", "--root", str(self.root),
                   "--binary", str(self.binary), "--evidence-dir", str(self.evidence),
                   "--tool-json", json.dumps({"name":"code_symbols","arguments":{"file":"source.ts"}})]
        result = subprocess.run(command, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "actual handler text\n")
        records = [json.loads(p.read_text()) for p in self.evidence.glob("*.json") if p.name != "setup.json"]
        self.assertEqual(len(records), 1)
        record = records[0]
        self.assertEqual(record["result"]["_meta"], {"receipt":"retained"})
        self.assertEqual(record["result"]["observed_args"], {"file":"source.ts"})
        self.assertEqual(set(record["timings_ms"]), {"guard","startup_initialize","identity","request","exit","total"})
        self.assertTrue(all(value >= 0 for value in record["timings_ms"].values()))
        self.assertFalse(record["source_changed_since_seed"])
        self.assertEqual(record["returncode"], 0)

    def test_duplicate_json_keys_are_rejected(self):
        with self.assertRaises(ValueError):
            bridge.strict_json('{"name":"code_search","name":"memory_search"}')

    def test_malformed_json_attempt_is_preserved_without_starting_mcp(self):
        self.prepare()
        with self.assertRaises(ValueError):
            bridge.call(self.root, self.binary, self.evidence, "{bad JSON")
        records = [json.loads(p.read_text()) for p in self.evidence.glob("*.json") if p.name != "setup.json"]
        self.assertEqual(records[0]["request"], "{bad JSON")
        self.assertEqual(records[0]["transcript"], [])
        self.assertTrue(records[0]["error"])

    def test_timeout_records_failure_and_reaps_process(self):
        (self.root / "stall-initialize").touch()
        self.prepare()
        with self.assertRaises(TimeoutError):
            bridge.call(self.root, self.binary, self.evidence,
                        {"name":"code_search","arguments":{"query":"x"}}, timeout=0.1)
        records = [json.loads(p.read_text()) for p in self.evidence.glob("*.json") if p.name != "setup.json"]
        record = records[0]
        self.assertTrue(record["forced_shutdown"])
        self.assertIsNotNone(record["returncode"])
        self.assertGreater(record["timings_ms"]["startup_initialize"], 0)
        self.assertIn("Timed out", record["error"])


@unittest.skipUnless(os.environ.get("AIDE_BRIDGE_TEST_BINARY"), "set AIDE_BRIDGE_TEST_BINARY for real MCP integration")
class RealMCPTests(unittest.TestCase):
    def test_all_five_actual_tools_isolation_and_seeded_staleness(self):
        with tempfile.TemporaryDirectory(prefix="aide-bridge-real-") as temporary:
            base = Path(temporary)
            root, evidence = base / "root", base / "evidence"
            root.mkdir()
            source = root / "fixture.ts"
            source.write_text("export function FixtureUnique(x: number): number {\n  return x + 1;\n}\nexport function FixtureCaller(): number {\n  return FixtureUnique(3);\n}\n")
            (base / "outside.ts").write_text("export function OutsideCanaryUnique() { return 9; }\n")
            binary = Path(os.environ["AIDE_BRIDGE_TEST_BINARY"])
            bridge.prepare(root, binary, evidence)
            def run(name, args):
                return bridge.text_contents(bridge.call(root, binary, evidence, {"name":name,"arguments":args}))
            for name, args, expected in [
                ("code_search", {"query":"FixtureUnique"}, "FixtureUnique"),
                ("code_references", {"symbol":"FixtureUnique"}, "FixtureUnique(3)"),
                ("code_symbols", {"file":"fixture.ts"}, "FixtureCaller"),
                ("code_outline", {"file":"fixture.ts"}, "FixtureUnique"),
                ("code_read_symbol", {"symbol":"FixtureUnique"}, "return x + 1"),
                ("code_read_symbol", {"symbols":["FixtureUnique", "FixtureCaller"]}, "return x + 1"),
                ("code_references", {"symbols":["FixtureUnique", "FixtureCaller"]}, "# Batch Reference Results"),
            ]:
                with self.subTest(tool=name, arguments=args):
                    self.assertIn(expected, run(name, args))
            self.assertIn("No matching symbols", run("code_search", {"query":"OutsideCanaryUnique"}))
            source.write_text(source.read_text().replace("FixtureUnique", "ChangedUnique").replace("x + 1", "x + 42"))
            self.assertIn("No matching symbols", run("code_search", {"query":"ChangedUnique"}))
            self.assertIn("ChangedUnique", run("code_symbols", {"file":"fixture.ts"}))
            self.assertIn("return x + 42", run("code_read_symbol", {"symbol":"ChangedUnique","file":"fixture.ts"}))
            records = [json.loads(p.read_text()) for p in evidence.glob("*.json") if p.name != "setup.json"]
            self.assertTrue(any(row["source_changed_since_seed"] for row in records))


if __name__ == "__main__":
    unittest.main()
