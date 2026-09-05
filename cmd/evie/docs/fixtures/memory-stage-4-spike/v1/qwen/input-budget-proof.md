# Verified Qwen2.5 BPE input budget

This proof applies only to the acquired `qwen2.5:7b-instruct-q4_K_M` artifact,
its stored Go template, the pinned Ollama 0.6.3 C++ runner, and the standalone
request shape. It is independent of the Mistral SPM proof. No inference result
is used to estimate the predispatch token budget.

[Actual tokenizer evidence](tokenizer-proof.json) comes from bounded GGUF
metadata inspection and a streaming SHA-256 of all 4,683,073,952 weight bytes.
The model digest is `2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730`;
the metadata region ends at 5,934,631 bytes. Inspection verified `qwen2`
architecture, `gpt2` tokenizer, `qwen2` preprocessor, 152,064 vocabulary entries,
all 256 encoded byte symbols, all 151,387 parsed merge concatenations present in
the vocabulary, and 22 active special tokens with nonempty spellings. Array and
metadata-region hashes make these observations reproducible using
[the inspection command](../../../../../../../scripts/memory-extractor-spike/inspect_qwen_gguf.py).

The owner independently checked the versioned runtime's byte map and BPE
implementation. Regex partitioning reconstructs original UTF-8 bytes and maps
each byte to one encoded symbol. Merges reduce the symbol count; verified merge
closure prevents the fallback over encoded UTF-8 from expanding it. Nonempty
specials consume source bytes and emit one token. At most one BOS and one EOS
are added. Therefore **B bytes of the complete rendered UTF-8 prompt require
at most B+2 input tokens**. The two added-token allowance is conservative even
where the model disables them.
([Byte map and reconstruction](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/unicode.cpp#L159-L255),
[BPE merge and fallback path](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/llama-vocab.cpp#L417-L587),
[merge parsing](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/llama-vocab.cpp#L1402-L1424))

Every installed model layer and the runtime executable/libraries are pinned in
[runtime-manifest.json](runtime-manifest.json). There are no stored messages,
parameter, adapter or projector layers. The 68-byte default system layer is
replaced by the nonempty request system string, not appended. The experiment
owned server explicitly sets `OLLAMA_NEW_ENGINE=false`, one model, one request,
loopback binding and disabled proxies. The current model-specific proof must
not be applied to an arbitrary server or changed model tag.

The spike executes the actual installed Go template, then checks exact equality
with this complete rendering (80 fixed UTF-8 bytes):

```text
"<|im_start|>system\n" + system + "<|im_end|>\n" +
"<|im_start|>user\n" + source_input + "<|im_end|>\n" +
"<|im_start|>assistant\n"
```

The request has one nonempty system string and one user prompt, no prior token
context, suffix, tools, images, raw mode or template override. The model's
stored GGUF Jinja template is not the selected Ollama Go template. The schema
format is a decoding constraint and adds no extra prompt copy.
([Generate and system override](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/server/routes.go#L240-L294),
[template execution/collation](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/template/template.go#L209-L303),
[engine and decoding path](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llm/server.go))

Before any inference, all selected windows and repetitions must pass source
validation, pinned prompt/schema/runtime/template/proof checks, exact rendering,
and this bound on the actual serialized input string:

`5588 system bytes + J input bytes +80 template bytes +2 +768 output +64 reserve <=8192`.

Thus J may be at most 1690 bytes. Public executable/local HTTP tests successfully
dispatch the exact 1690-byte boundary and reject 1691 bytes before any request.
They also reject changed model/template, missing proof and changed prompt;
shared full-batch preflight rejects an oversized later window before an earlier
valid one is dispatched. The extra 64 tokens are reserve, not a substitute for
counting rendering. The first scheduled quality request is the load/protocol
smoke; `-stop-on-failure` preserves any failed attempt and the unexecuted plan.
Source-check rejection or a valid but incorrect proposal remains quality evidence.

The actual runner path/context and resource behavior still require observation
when the model first loads. Output truncation remains a failed response; a
post-response token count is not permission for silent input truncation. These
standalone bounds and local artifacts do not implement #136's production budget.
