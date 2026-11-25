"""Minimal Ollama HTTP client using only the Python standard library."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Any, Dict, Iterable, Iterator, List, Optional

from . import config


class OllamaError(RuntimeError):
    """A friendly error for connection and API failures."""


def _base(cfg: Optional[Dict[str, Any]] = None) -> str:
    return config.ollama_url(cfg or config.load())


def _payload(method: str, path: str, data: Optional[Dict[str, Any]] = None, timeout: float = 30.0):
    url = _base() + path
    body = json.dumps(data).encode("utf-8") if data is not None else None
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header("Accept", "application/json")
    if body is not None:
        req.add_header("Content-Type", "application/json")
    try:
        return urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", "replace").strip()
        raise OllamaError(f"Ollama returned HTTP {exc.code}: {detail[:300]}") from exc
    except urllib.error.URLError as exc:
        reason = getattr(exc, "reason", exc)
        raise OllamaError(
            f"cannot reach Ollama at {_base()} ({reason}). Start it with `ollama serve`."
        ) from exc
    except TimeoutError as exc:
        raise OllamaError(f"Ollama timed out at {_base()}") from exc


def _json(method: str, path: str, data: Optional[Dict[str, Any]] = None, timeout: float = 30.0) -> Dict[str, Any]:
    with _payload(method, path, data, timeout) as resp:
        raw = resp.read().decode("utf-8")
    try:
        return json.loads(raw)
    except json.JSONDecodeError as exc:
        raise OllamaError(f"invalid JSON from Ollama: {raw[:200]}") from exc


def version(cfg: Optional[Dict[str, Any]] = None) -> str:
    try:
        return str(_json("GET", "/api/version", timeout=4).get("version", "unknown"))
    except OllamaError:
        raise
    except Exception as exc:  # noqa: BLE001 - surface a single friendly error
        raise OllamaError(str(exc)) from exc


def list_models(cfg: Optional[Dict[str, Any]] = None) -> List[Dict[str, Any]]:
    data = _json("GET", "/api/tags")
    models = data.get("models") or []
    models.sort(key=lambda m: str(m.get("name", "")).lower())
    return models


def show(name: str) -> Dict[str, Any]:
    return _json("POST", "/api/show", {"model": name})


def delete(name: str) -> None:
    with _payload("DELETE", "/api/delete", {"model": name}) as resp:
        resp.read()


def pull(name: str, on_progress=None) -> None:
    """Download a model, invoking on_progress(dict) for every streamed status."""
    with _payload("POST", "/api/pull", {"model": name, "stream": True}, timeout=600) as resp:
        for line in resp:
            line = line.strip()
            if not line:
                continue
            try:
                status = json.loads(line)
            except json.JSONDecodeError:
                continue
            if status.get("error"):
                raise OllamaError(str(status["error"]))
            if on_progress:
                on_progress(status)


def running(cfg: Optional[Dict[str, Any]] = None) -> List[Dict[str, Any]]:
    try:
        return list(_json("GET", "/api/ps").get("models") or [])
    except OllamaError:
        return []


def chat_stream(
    model: str,
    messages: Iterable[Dict[str, Any]],
    tools: Optional[List[Dict[str, Any]]] = None,
    options: Optional[Dict[str, Any]] = None,
) -> Iterator[Dict[str, Any]]:
    """Yield JSON chunks from /api/chat."""
    payload: Dict[str, Any] = {
        "model": model,
        "messages": list(messages),
        "stream": True,
    }
    if tools:
        payload["tools"] = tools
    if options:
        payload["options"] = options
    with _payload("POST", "/api/chat", payload, timeout=600) as resp:
        for line in resp:
            line = line.strip()
            if not line:
                continue
            try:
                chunk = json.loads(line)
            except json.JSONDecodeError:
                continue
            if chunk.get("error"):
                raise OllamaError(str(chunk["error"]))
            yield chunk


def human_size(num: float) -> str:
    if not num:
        return "—"
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if num < 1024 or unit == "TB":
            return f"{num:.1f} {unit}" if unit != "B" else f"{int(num)} B"
        num /= 1024
    return f"{num:.1f} TB"


def resolve_model(cfg: Dict[str, Any], explicit: Optional[str]) -> str:
    if explicit:
        return explicit
    if cfg.get("model"):
        return str(cfg["model"])
    try:
        models = list_models(cfg)
    except OllamaError:
        models = []
    if models:
        return str(models[0]["name"])
    raise OllamaError(
        "no model selected. Run `monet models pull qwen2.5-coder:7b` or `monet models use <name>`."
    )
