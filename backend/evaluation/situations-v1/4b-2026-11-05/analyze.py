#!/usr/bin/env python3
"""Compare reviewed 4B runs to the preserved 2B experiment."""
import hashlib,json,math,pathlib,statistics
HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parent
CASES={c['id']:c for c in json.loads((ROOT/'build-recovered-v3/questions.json').read_text())}
MAP={'control-local':'control-local','A-local':'A-branchsafe-local','B-local':'B-local','control-oracle':'control-oracle','B-oracle':'B-oracle'}
def read(p):return json.loads(p.read_text())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def frac(n,d):return {'numerator':n,'denominator':d,'value':n/d if d else None}
def stat(xs):return {'n':len(xs),'p50':statistics.median(xs),'p95_nearest_rank':sorted(xs)[math.ceil(.95*len(xs))-1],'min':min(xs),'max':max(xs)} if xs else None
def scores(rs,rv):
 out={};by={r['case']:r for r in rv}
 for split in [None,'existing-regression','independent-source-diagnostic']:
  xs=[r for r in rs if split is None or CASES[r['case']]['split']==split];vs=[by[r['case']] for r in xs];ans=[r for r in vs if CASES[r['case']]['evidence']];unans=[r for r in vs if not CASES[r['case']]['evidence']]
  m={k:frac(sum(r[k] for r in vs),len(vs)) for k in ['task_pass','strict_pass','unsupported_addition','spoiler']}
  m['episode_error_answerable']=frac(sum(r['episode_error'] for r in ans),len(ans));m['safe_unanswerable']=frac(sum(r['task_pass'] for r in unans),len(unans))
  for k in ['exact_code','exact_sequence']:m[k]=frac(sum(r[k] for r in vs if k in r),sum(k in r for r in vs))
  m['runtime_success']=frac(sum(not r.get('error') for r in xs),len(xs));out[split or 'all']=m
 out['context_tokens']=stat([r['debug']['context_tokens'] for r in rs]);out['generation_seconds_including_retry']=stat([r['debug']['stage_ms']['generation']/1000 for r in rs]);out['total_seconds_cached_embeddings']=stat([r['debug']['stage_ms']['total']/1000 for r in rs]);out['prompt_tokens_last_attempt']=stat([r['debug']['completion_attempts'][-1]['prompt_tokens'] for r in rs if r['debug']['completion_attempts']]);return out
def main():
 result={'schema':'situations-4b-comparison-v1','model':'Qwen3.5-4B UD-Q4_K_XL','seed':20261104,'profiles':['default'],'context_token_cap':2400,'runtime_build':'b11706-f74e2fa6a','old_artifacts_unchanged':True,'comparability':{},'generation':{},'paired':{}}
 for relative,expected in read(HERE/'preservation-before.json').items():assert sha(ROOT/relative)==expected,relative
 for name,old_name in MAP.items():
  path=HERE/'runs'/name;old=ROOT/'runs'/old_name
  rs=read(path/'results.json');previous=read(old/'results.json');rv=read(path/'review.json')['records'];old_rv=read(old/'review.json')['records'];m=read(path/'manifest.json');om=read(old/'manifest.json')
  for k in ['cases_sha256','corpus_sha256','source_sha256','embedding_profile','embedding_runtime','kind','seed','limit','profiles','modes','temperature','max_tokens','retry_max_tokens','context_token_budget','no_spoilers','answer_length','enable_thinking']:assert m[k]==om[k],(name,k)
  assert m['generation_runtime']['default_generation_settings']==om['generation_runtime']['default_generation_settings']
  assert m['generation_runtime']['build_info']==om['generation_runtime']['build_info']
  assert m['generation_runtime']['chat_template']==om['generation_runtime']['chat_template']
  old_by={r['case']:r for r in previous}
  for r in rs:
   p=old_by[r['case']]
   # Exact evidence text, ranking, token counts and messages: change model only.
   for k in ['search_input','context_text','context_tokens','search','retrieved_chunks']:assert r['debug'][k]==p['debug'][k],(name,r['case'],k)
   a=r['debug']['model_request'];b=p['debug']['model_request']
   for k in ['messages','temperature','chat_template_kwargs','seed']:assert a[k]==b[k],(name,r['case'],k)
  if (old/'query-embeddings.json').exists():assert read(path/'query-embeddings.json')==read(old/'query-embeddings.json')
  result['comparability'][name]={'cases':len(rs),'same_corpus_sha256':m['corpus_sha256'],'same_context_text_ranking_and_tokens':True,'same_messages':True,'same_generation_defaults':True,'same_query_vectors':True}
  result['generation'][name]={'4b':scores(rs,rv),'2b':scores(previous,old_rv)}
  pby={r['case']:r for r in old_rv};result['paired'][name]={'improved_4b_vs_2b':[r['case'] for r in rv if r['task_pass'] and not pby[r['case']]['task_pass']],'regressed_4b_vs_2b':[r['case'] for r in rv if not r['task_pass'] and pby[r['case']]['task_pass']]}
 control={r['case']:r for r in read(HERE/'runs/control-local/review.json')['records']}
 for v in ['A','B']:
  rv=read(HERE/'runs'/(v+'-local')/'review.json')['records'];result['paired'][v+'-vs-control-4b']={'improved':[r['case'] for r in rv if r['task_pass'] and not control[r['case']]['task_pass']],'regressed':[r['case'] for r in rv if not r['task_pass'] and control[r['case']]['task_pass']]}
 result['langfuse']={'items':0,'scores':0,'experiments':[],'project':'http://localhost:3000/project/walkthrough-backend'}
 for name in MAP:
  states=list((HERE/'runs'/name).glob('.langfuse-*.json'))
  if not states:continue
  assert len(states)==1;s=read(states[0]);result['langfuse']['items']+=s['records'];result['langfuse']['scores']+=len(s['scores']);result['langfuse']['experiments']+=s['experiments']
 out=HERE/'comparison.json'
 if out.exists():assert read(out)==result
 else:out.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
 for n,m in result['generation'].items():print(n,'2B',m['2b']['all'],'4B',m['4b']['all'],'time4b',m['4b']['generation_seconds_including_retry'])
 print('paired',result['paired']);print('Langfuse',result['langfuse']['items'],result['langfuse']['scores'])
if __name__=='__main__':main()
