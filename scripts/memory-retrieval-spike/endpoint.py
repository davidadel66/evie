"""Bounded local-only embedding transport used by this disposable experiment."""
import asyncio
import ipaddress
import json
import math
from urllib.parse import urlsplit

class LocalEmbedding:
    def __init__(self, endpoint, model="all-minilm:22m", dimensions=384):
        self.model, self.dimensions = model, dimensions
        if endpoint.startswith("unix:"):
            self.socket_path = endpoint[5:]
            if not self.socket_path.startswith("/"):
                raise ValueError("absolute Unix socket required")
            self.host, self.port = "localhost", None
        else:
            parsed = urlsplit(endpoint)
            if parsed.scheme != "http" or parsed.username or parsed.password or parsed.path not in ("", "/") or parsed.query or parsed.fragment:
                raise ValueError("plain loopback endpoint required")
            if not ipaddress.ip_address(parsed.hostname).is_loopback:
                raise ValueError("loopback address required")
            self.host, self.port, self.socket_path = parsed.hostname, parsed.port or 80, None

    async def _request(self, payload):
        if self.socket_path:
            reader, writer = await asyncio.open_unix_connection(self.socket_path, limit=65536)
        else:
            reader, writer = await asyncio.open_connection(self.host, self.port, limit=65536)
        try:
            body = json.dumps(payload, allow_nan=False).encode()
            writer.write(f"POST /api/embed HTTP/1.1\r\nHost: {self.host}\r\nContent-Type: application/json\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n".encode() + body)
            await writer.drain()
            headers = await reader.readuntil(b"\r\n\r\n")
            lines = headers.decode("ascii").split("\r\n")
            if lines[0].split(" ")[1] != "200":
                raise ValueError("embedding HTTP status rejected; redirects are never followed")
            values = dict(line.lower().split(":", 1) for line in lines[1:] if ":" in line)
            if "transfer-encoding" in values:
                if values["transfer-encoding"].strip() != "chunked":
                    raise ValueError("unknown HTTP transfer encoding")
                body = bytearray()
                for _ in range(4096):
                    length = int((await reader.readuntil(b"\r\n")).strip(), 16)
                    if length < 0 or len(body) + length > 4 * 1024 * 1024:
                        raise ValueError("embedding response bound")
                    if length == 0:
                        break
                    body.extend(await reader.readexactly(length))
                    if await reader.readexactly(2) != b"\r\n":
                        raise ValueError("invalid HTTP chunk")
                else:
                    raise ValueError("HTTP chunk count bound")
            else:
                length = int(values.get("content-length", "-1"))
                if length < 0 or length > 4 * 1024 * 1024:
                    raise ValueError("bounded content length required")
                body = await reader.readexactly(length)
            return json.loads(body, parse_constant=lambda _: (_ for _ in ()).throw(ValueError("nonfinite JSON")))
        finally:
            writer.close()
            await writer.wait_closed()

    async def embed_async(self, texts, timeout=10):
        if not texts or len(texts) > 32 or any(not isinstance(t, str) or len(t.encode()) > 1024 for t in texts):
            raise ValueError("embedding input bound")
        result = await asyncio.wait_for(self._request({"model": self.model, "input": texts, "truncate": False, "keep_alive": "30s", "options": {"num_thread": 4}}), timeout)
        vectors = result.get("embeddings")
        if result.get("model") != self.model or not isinstance(vectors, list) or len(vectors) != len(texts):
            raise ValueError("embedding model/count mismatch")
        for vector in vectors:
            if not isinstance(vector, list) or len(vector) != self.dimensions or any(type(x) not in (int, float) or not math.isfinite(x) for x in vector):
                raise ValueError("embedding shape/number mismatch")
            norm = math.sqrt(sum(x*x for x in vector))
            if norm < 1e-12:
                raise ValueError("zero embedding")
            for i, value in enumerate(vector):
                vector[i] = value / norm
        return vectors

    def embed(self, texts, timeout=10):
        return asyncio.run(self.embed_async(texts, timeout))
