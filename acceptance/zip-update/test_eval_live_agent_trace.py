#!/usr/bin/env python3
"""Adversarial tests for the fail-closed Yoda live-Agent trace evaluator."""

from __future__ import annotations

import unittest
import os
import subprocess
import tempfile
import shutil
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from eval_live_agent_trace import evaluate  # noqa: E402


def text(value: str) -> dict[str, str]:
    return {"type": "text", "text": value}


def correlated_trace() -> list[dict]:
    return [
        {"type": "system", "subtype": "hook_response", "hook_event": "SessionStart", "hook_id": "session-start-scaffold", "exit_code": 0, "outcome": "success"},
        {"type": "system", "subtype": "hook_response", "hook_event": "SessionStart", "hook_id": "session-start-memory", "exit_code": 0, "outcome": "success"},
        {"type": "system", "subtype": "init", "cwd": "/tmp/Maestro", "agents": ["yoda"], "slash_commands": ["maestro-setup-update"]},
        {"type": "system", "subtype": "hook_response", "hook_event": "UserPromptSubmit", "exit_code": 0, "output": "<!-- maestro:agent-route -->\n- `yoda`"},
        {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "toolu_yoda", "name": "Agent", "input": {"subagent_type": "yoda"}}]}},
        {"type": "system", "subtype": "hook_response", "hook_event": "PreToolUse", "exit_code": 0, "output": "dispatch yoda"},
        {"type": "assistant", "parent_tool_use_id": "toolu_yoda", "message": {"content": [text("QUALIFICACAO_YODA_OK")]}},
        {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": "toolu_yoda", "content": "QUALIFICACAO_YODA_OK"}]}},
        {"type": "assistant", "message": {"content": [text("QUALIFICACAO_MAESTRO_OK")]}},
        {"type": "system", "subtype": "hook_response", "hook_event": "Stop", "exit_code": 0, "outcome": "success"},
    ]


class EvaluateTraceTest(unittest.TestCase):
    def test_prompt_tokens_do_not_fake_agent_calls(self):
        checks, tool_use_ids = evaluate([
            {"type": "user", "message": {"content": [text("yoda QUALIFICACAO_YODA_OK")]}},
            {"type": "assistant", "message": {"content": [text("QUALIFICACAO_YODA_OK\nQUALIFICACAO_MAESTRO_OK")]}},
        ], 0)
        self.assertEqual(tool_use_ids, {"yoda": None})
        self.assertFalse(checks["exactly_one_agent_tool_yoda_observed"])
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])

    def test_correlated_yoda_trace_passes(self):
        checks, tool_use_ids = evaluate(correlated_trace(), 0)
        self.assertEqual(tool_use_ids, {"yoda": "toolu_yoda"})
        self.assertTrue(all(checks.values()), checks)

    def test_route_hook_error_fails(self):
        events = correlated_trace()
        events[3]["outcome"] = "error"
        events[3]["exit_code"] = 2
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["user_prompt_route_hook_returned_yoda"])

    def test_pretool_hook_error_fails(self):
        events = correlated_trace()
        events[5]["outcome"] = "error"
        events[5]["exit_code"] = 2
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_pretool_hook_correlated"])

    def test_extra_agent_call_fails_exact_contract(self):
        events = correlated_trace()
        events.insert(-1, {"type": "assistant", "message": {"content": [{"type": "tool_use", "id": "toolu_extra", "name": "Agent", "input": {"subagent_type": "gamma-guardian"}}]}})
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["exactly_one_agent_tool_call_total"])

    def test_mismatched_result_id_fails(self):
        events = correlated_trace()
        events[6]["parent_tool_use_id"] = "toolu_wrong"
        events[7]["message"]["content"][0]["tool_use_id"] = "toolu_wrong"
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["yoda_return_correlated_to_tool_use"])
        self.assertFalse(checks["hub_return_after_yoda_result"])

    def test_one_session_start_hook_is_not_enough(self):
        events = correlated_trace()
        del events[1]
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["both_session_start_hooks_succeeded"])

    def test_duplicate_session_start_hook_id_is_not_enough(self):
        events = correlated_trace()
        events[1]["hook_id"] = events[0]["hook_id"]
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["both_session_start_hooks_succeeded"])

    def test_missing_project_init_fails(self):
        events = correlated_trace()
        del events[2]
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["project_init_loaded_yoda_and_update_contract"])

    def test_missing_stop_hook_fails(self):
        events = correlated_trace()
        events.pop()
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["stop_hook_succeeded_after_hub"])

    def test_stop_before_hub_return_fails(self):
        events = correlated_trace()
        stop = events.pop()
        events.insert(-1, stop)
        checks, _ = evaluate(events, 0)
        self.assertFalse(checks["stop_hook_succeeded_after_hub"])


class LiveQualificationProfileTests(unittest.TestCase):
    def test_missing_or_unsafe_profile_stops_before_model_and_state_effects(self):
        self.check_profile_refusals(False)

    @unittest.skipUnless(shutil.which("pwsh"), "PowerShell source parser unavailable")
    def test_powershell_profile_refusals_precede_model_and_native_admission(self):
        self.check_profile_refusals(True)

    def check_profile_refusals(self, powershell):
        script = Path(__file__).with_name("live-agent-qualification.ps1" if powershell else "live-agent-qualification.sh")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            current = root / "operator-profile"
            current.mkdir(mode=0o700)
            dedicated = root / "dedicated-profile"
            dedicated.mkdir(mode=0o700)
            alias = root / "profile-alias"
            alias.symlink_to(dedicated, target_is_directory=True)
            commands = root / "bin"
            commands.mkdir()
            invoked = root / "model-invoked"
            (commands / "claude").write_text('#!/bin/sh\nprintf invoked > "$MODEL_MARKER"\n', encoding="utf-8")
            (commands / "claude").chmod(0o755)
            archive = root / "candidate.zip"
            archive.write_bytes(b"profile validation must precede extraction")
            trace, receipt = root / "trace.jsonl", root / "receipt.json"
            environment = dict(os.environ, HOME=str(current), USERPROFILE=str(current),
                               XDG_CACHE_HOME=str(root / "shell-host-cache"),
                               PATH=str(commands) + os.pathsep + os.environ["PATH"],
                               MODEL_MARKER=str(invoked), MAESTRO_HOME=str(root / "outside-owner"))
            # PowerShell startup owns these files independently of the script under test.
            if powershell:
                environment.update(XDG_CACHE_HOME=str(root / "shell-host/cache"),
                                   XDG_DATA_HOME=str(root / "shell-host/data"),
                                   XDG_CONFIG_HOME=str(root / "shell-host/config"))
            environment.pop("CLAUDE_CONFIG_DIR", None)
            (current / "sentinel.txt").write_text("preserved operator state", encoding="utf-8")
            operator_before = {str(path.relative_to(current)): None if path.is_dir() else path.read_bytes()
                               for path in current.rglob("*")}
            cases = [([], {}), (["--profile", "relative-profile"], {}),
                     (["--profile", str(alias)], {}), (["--profile", str(current)], {}),
                     (["--profile", str(dedicated)], {"CLAUDE_CONFIG_DIR": str(current / ".claude")})]
            for arguments, overrides in cases:
                with self.subTest(arguments=arguments, overrides=overrides):
                    if powershell:
                        profile_args = ["-Profile" if value == "--profile" else value for value in arguments]
                        command = ["pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-File", str(script),
                                   "-Zip", str(archive), "-Trace", str(trace), "-Receipt", str(receipt), *profile_args]
                    else:
                        command = ["bash", str(script), "--zip", str(archive),
                                   "--trace", str(trace), "--receipt", str(receipt), *arguments]
                    result = subprocess.run(command, env={**environment, **overrides}, text=True, capture_output=True)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn("profile", result.stderr.lower())
                    self.assertFalse(invoked.exists())
                    self.assertFalse(trace.exists())
                    self.assertFalse(receipt.exists())
                    self.assertEqual({str(path.relative_to(current)): None if path.is_dir() else path.read_bytes()
                                      for path in current.rglob("*")}, operator_before)
                    self.assertEqual(list(dedicated.iterdir()), [])
                    self.assertFalse((root / "outside-owner").exists())


if __name__ == "__main__":
    unittest.main()
