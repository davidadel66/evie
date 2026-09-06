from pathlib import Path
import hashlib,json
from concurrent.futures import ThreadPoolExecutor
from urllib.request import urlopen
root=Path(__file__).parent
commit='e5d84fb90b21d71f8eb816656ca0b34191425216'
paths={
'common.cpp':'llama/llama.cpp/common/common.cpp',
'common.h':'llama/llama.cpp/common/common.h',
'json-schema-to-grammar.cpp':'llama/llama.cpp/common/json-schema-to-grammar.cpp',
'json-schema-to-grammar.h':'llama/llama.cpp/common/json-schema-to-grammar.h',
'json.hpp':'llama/llama.cpp/common/json.hpp',
'llama-cpp.h':'llama/llama.cpp/include/llama-cpp.h',
'llama.h':'llama/llama.cpp/include/llama.h',
'llama-grammar.cpp':'llama/llama.cpp/src/llama-grammar.cpp',
'llama-grammar.h':'llama/llama.cpp/src/llama-grammar.h',
'llama-impl.h':'llama/llama.cpp/src/llama-impl.h',
'llama-vocab.h':'llama/llama.cpp/src/llama-vocab.h',
'llama-sampling.h':'llama/llama.cpp/src/llama-sampling.h',
'llama__llama.go':'llama/llama.go',
'llama__sampling_ext.cpp':'llama/sampling_ext.cpp',
'llama__sampling_ext.h':'llama/sampling_ext.h',
'llama__common.go':'llama/llama.cpp/common/common.go',
'CMakeLists.txt':'CMakeLists.txt',
}
for name in ['ggml.h','ggml-cpu.h','ggml-backend.h','ggml-alloc.h']:
    paths[name]='ml/backend/ggml/ggml/include/'+name

def verify(item):
    name,path=item
    url='https://raw.githubusercontent.com/ollama/ollama/'+commit+'/'+path
    data=urlopen(url).read()
    assert data==(root/'source'/name).read_bytes(),name
    return {'local_name':name,'repository_path':path,'url':url,'sha256':hashlib.sha256(data).hexdigest(),'bytes':len(data)}
with ThreadPoolExecutor(max_workers=8) as pool:sources=list(pool.map(verify,sorted(paths.items())))
manifest={'repository':'https://github.com/ollama/ollama','tag':'v0.6.3','commit':commit,'tag_ref_url':'https://api.github.com/repos/ollama/ollama/git/ref/tags/v0.6.3','tested_schema_sha256':hashlib.sha256((root/'output.schema.json').read_bytes()).hexdigest(),'sources':sources}
(root/'sources.json').write_text(json.dumps(manifest,indent=2)+'\n')
print('Verified immutable-commit bytes for',len(sources),'source files')
