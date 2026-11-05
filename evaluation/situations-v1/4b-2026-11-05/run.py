#!/usr/bin/env python3
"""Repeat the frozen situations experiment with only the generator changed."""
import hashlib,json,pathlib,shutil,subprocess,urllib.request
HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parent
BACKEND=ROOT.parent.parent

def digest(p):
 h=hashlib.sha256()
 with p.open('rb') as f:
  while b:=f.read(1024*1024):h.update(b)
 return h.hexdigest()
def props(port):
 with urllib.request.urlopen(f'http://127.0.0.1:{port}/props',timeout=10) as r:return json.load(r)
def write_new(p,data):
 p.parent.mkdir(parents=True,exist_ok=True)
 with p.open('x') as f:json.dump(data,f,ensure_ascii=False,indent=2);f.write('\n')
def main():
 (HERE/'runtime').mkdir(exist_ok=True)
 original=json.loads((ROOT/'runs/B-local/manifest.json').read_text())
 generation=props(18083);embedding=props(18082)
 assert generation['build_info']==original['generation_runtime']['build_info']
 assert generation['default_generation_settings']==original['generation_runtime']['default_generation_settings'],'samplers/runtime differ from 2B'
 assert embedding['default_generation_settings']==original['embedding_runtime']['default_generation_settings']
 assert digest(pathlib.Path(embedding['model_path']))==original['embedding_profile']['gguf_sha256']
 assert digest(pathlib.Path(generation['model_path']))=='b252c5610a42ca82d20fe2a12813e9d069eed89292907e26c783eeb0bc961bc7'
 assert 'enable_thinking' in generation['chat_template']
 for name,expected in original['source_sha256'].items():assert digest(BACKEND/name)==expected,name
 protected={}
 for run in ['control-local','A-branchsafe-local','B-local','control-oracle','B-oracle']:
  for file in ['manifest.json','results.json','review.json','summary.json']:
   p=ROOT/'runs'/run/file;protected[str(p.relative_to(ROOT))]=digest(p)
 for v in ['control','A','B']:
  p=ROOT/'build-recovered-v3'/v/'dino-crisis-1.rag';protected[str(p.relative_to(ROOT))]=digest(p)
 for file in ['REPORT.md','comparison.json','artifact.json']:
  p=ROOT/file;protected[file]=digest(p)
 write_new(HERE/'preservation-before.json',protected)
 write_new(HERE/'runtime-preflight.json',{'generation_props':generation,'embedding_props':embedding,'generation_sha256':digest(pathlib.Path(generation['model_path'])),'identical_generation_defaults_to_2b':True,'identical_go_sources_to_2b':True,'query_embeddings':'copied exact cache from original 2B run; no new embedding inference'})
 failures=[]
 for variant,kind,old in [('control','local','control-local'),('A','local','A-branchsafe-local'),('B','local','B-local'),('control','oracle','control-oracle'),('B','oracle','B-oracle')]:
  out=HERE/'runs'/(variant+'-'+kind)
  if out.exists():raise SystemExit('Choose a fresh experiment directory; old results are immutable')
  out.mkdir(parents=True)
  cache=ROOT/'runs'/old/'query-embeddings.json'
  # Oracle has no query embedding cache and never calls the embedder.
  if cache.exists():shutil.copyfile(cache,out/'query-embeddings.json')
  cmd=['go','run','./cmd/prompt-eval','--kind',kind,'--corpus',str(ROOT/'build-recovered-v3'/variant/'dino-crisis-1.rag'),'--cases',str(ROOT/'build-recovered-v3/questions.json'),'--embedding-url','http://127.0.0.1:18082','--llama-url','http://127.0.0.1:18083','--model','assistance-4b','--context-tokens','2400','--profiles','default','--modes','hybrid','--seed','20261104','--out',str(out)]
  print('RUN',variant,kind,flush=True)
  with (HERE/'runtime'/(variant+'-'+kind+'.log')).open('x') as f:result=subprocess.run(cmd,cwd=BACKEND,stdout=f,stderr=subprocess.STDOUT)
  print('DONE',variant,kind,'exit',result.returncode,flush=True)
  if result.returncode:failures.append(variant+'-'+kind)
 for name,expected in protected.items():assert digest(ROOT/name)==expected,name
 write_new(HERE/'run-completion.json',{'failed_runs':failures,'old_artifacts_preserved':True})
 if failures:raise SystemExit('Cells failed and retained in: '+','.join(failures))
if __name__=='__main__':main()
