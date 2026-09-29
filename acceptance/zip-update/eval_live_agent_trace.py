#!/usr/bin/env python3
"""Correlate a Claude stream trace into release-qualification evidence."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def _blocks(event: dict[str, Any]) -> list[dict[str, Any]]:
    message = event.get("message") or {}
    content = message.get("content") or []
    return content if isinstance(content, list) else []


def _text_from(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return "\n".join(_text_from(item) for item in value)
    if isinstance(value, dict):
        if value.get("type") == "text":
            return str(value.get("text") or "")
        return "\n".join(_text_from(item) for item in value.values())
    return ""


def _has_exact_line(value: Any, token: str) -> bool:
    return any(line.strip() == token for line in _text_from(value).splitlines())


def evaluate(
    events: list[dict[str, Any]], claude_rc: int
) -> tuple[dict[str, bool], dict[str, str | None]]:
    """Return fail-closed checks and the observed Yoda tool-use id."""
    route_hook = False
    session_start_hook_ids: set[str] = set()
    stop_hook_indexes: list[int] = []
    project_init = False
    agent_calls: list[tuple[int, str | None]] = []
    all_agent_calls: list[tuple[int, str | None, str | None]] = []

    for index, event in enumerate(events):
        if event.get("type") == "system" and event.get("subtype") == "hook_response":
            hook_event = event.get("hook_event")
            hook_text = "\n".join(str(event.get(key) or "") for key in ("output", "stdout"))
            if (
                hook_event == "SessionStart"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
            ):
                hook_id = str(event.get("hook_id") or "")
                if hook_id:
                    session_start_hook_ids.add(hook_id)
            if (
                hook_event == "UserPromptSubmit"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
                and "<!-- maestro:agent-route -->" in hook_text
                and "`yoda`" in hook_text
            ):
                route_hook = True
            if (
                hook_event == "Stop"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
            ):
                stop_hook_indexes.append(index)
        if event.get("type") == "system" and event.get("subtype") == "init":
            cwd = Path(str(event.get("cwd") or ""))
            agents = event.get("agents") or []
            commands = event.get("slash_commands") or []
            project_init = (
                cwd.name == "Maestro"
                and "yoda" in agents
                and "maestro-setup-update" in commands
            )
        if event.get("type") == "assistant":
            for block in _blocks(event):
                agent_input = block.get("input") or {}
                if block.get("type") == "tool_use" and block.get("name") == "Agent":
                    all_agent_calls.append(
                        (index, block.get("id"), agent_input.get("subagent_type"))
                    )
                if (
                    block.get("type") == "tool_use"
                    and block.get("name") == "Agent"
                    and agent_input.get("subagent_type") == "yoda"
                ):
                    agent_calls.append((index, block.get("id")))

    call_index, call_id = agent_calls[0] if len(agent_calls) == 1 else (-1, None)
    result_index = -1
    agent_return = False
    if call_id:
        for index, event in enumerate(events):
            if (
                event.get("type") == "assistant"
                and event.get("parent_tool_use_id") == call_id
                and _has_exact_line(_blocks(event), "QUALIFICACAO_YODA_OK")
            ):
                agent_return = True
                result_index = max(result_index, index)
            if event.get("type") == "user":
                for block in _blocks(event):
                    if (
                        block.get("type") == "tool_result"
                        and block.get("tool_use_id") == call_id
                    ):
                        result_index = max(result_index, index)
                        if _has_exact_line(block.get("content"), "QUALIFICACAO_YODA_OK"):
                            agent_return = True

    pretool_hook = False
    if call_index >= 0:
        upper = result_index if result_index >= 0 else len(events) - 1
        for event in events[call_index + 1 : upper + 1]:
            if (
                event.get("type") == "system"
                and event.get("subtype") == "hook_response"
                and event.get("hook_event") == "PreToolUse"
                and event.get("outcome") != "error"
                and event.get("exit_code", 0) == 0
            ):
                hook_text = "\n".join(
                    str(event.get(key) or "") for key in ("output", "stdout")
                )
                if "yoda" in hook_text:
                    pretool_hook = True

    hub_return = False
    hub_return_index = -1
    if result_index >= 0:
        for index, event in enumerate(events[result_index + 1 :], start=result_index + 1):
            if (
                event.get("type") == "assistant"
                and not event.get("parent_tool_use_id")
                and _has_exact_line(_blocks(event), "QUALIFICACAO_MAESTRO_OK")
            ):
                hub_return = True
                hub_return_index = index

    checks = {
        "claude_exit_zero": claude_rc == 0,
        "both_session_start_hooks_succeeded": len(session_start_hook_ids) >= 2,
        "project_init_loaded_yoda_and_update_contract": project_init,
        "user_prompt_route_hook_returned_yoda": route_hook,
        "exactly_one_agent_tool_call_total": len(all_agent_calls) == 1,
        "exactly_one_agent_tool_yoda_observed": len(agent_calls) == 1,
        "yoda_pretool_hook_correlated": pretool_hook,
        "yoda_return_correlated_to_tool_use": agent_return,
        "hub_return_after_yoda_result": hub_return,
        "stop_hook_succeeded_after_hub": hub_return_index >= 0
        and any(index > hub_return_index for index in stop_hook_indexes),
    }
    return checks, {"yoda": call_id}


def _load_events(path: Path) -> list[dict[str, Any]]:
    events = []
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            events.append(value)
    return events


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--trace", type=Path, required=True)
    parser.add_argument("--receipt", type=Path, required=True)
    parser.add_argument("--platform", required=True)
    parser.add_argument("--architecture", required=True)
    parser.add_argument("--release-sha256", required=True)
    parser.add_argument("--claude-version", required=True)
    parser.add_argument("--claude-rc", type=int, required=True)
    args = parser.parse_args()

    checks, agent_call_ids = evaluate(_load_events(args.trace), args.claude_rc)
    verdict = "PASS" if all(checks.values()) else "FAIL"
    receipt = {
        "schema_version": 1,
        "evidence_kind": "maestro_claude_agent_release_qualification",
        "platform": args.platform,
        "architecture": args.architecture,
        "release_sha256": args.release_sha256,
        "claude_code": args.claude_version,
        "agent_tool_use_ids": agent_call_ids,
        "checks": checks,
        "verdict": verdict,
        "limits": [
            "controlled qualification prompt",
            "not release publication or signing evidence",
        ],
    }
    with args.receipt.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(receipt, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    print(verdict)
    return 0 if verdict == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
