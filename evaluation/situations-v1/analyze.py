#!/usr/bin/env python3
import pathlib,json,statistics,math
ROOT=pathlib.Path(__file__).resolve().parent
cases={c['id']:c for c in json.loads((ROOT/'build-recovered-v3/questions.json').read_text())}
def read(p):return json.loads(p.read_text())
def match(e,r):
 s=e['source_span'];spans=r.get('source_spans',[])
 return any(t['document_sha256']==s['document_sha256'] and t['start']<=s['start'] and t['end']>=s['end'] for t in spans) if spans else r.get('document_sha256')==s['document_sha256'] and s['quote'] in r['Text']
def frac(n,d):return {'numerator':n,'denominator':d,'value':None if not d else n/d}
def stats(xs):
 return {'n':len(xs),'p50':statistics.median(xs),'p95_nearest_rank':sorted(xs)[math.ceil(.95*len(xs))-1],'min':min(xs),'max':max(xs)} if xs else None
summary={'protocol':{'cases':38,'answerable':34,'unanswerable_or_ambiguous':4,'source':'recovered baseline chunks; NOT original .rag','hardware':'Apple M1 Pro, 32 GiB; no Steam Deck measurements','prompt':'default','generation':'Qwen3.5-2B UD-Q4_K_XL','runtime':'b11706-f74e2fa6a','seed':20261104,'temperature':.2,'max_tokens':256,'retry_max_tokens':512,'limit':5,'context_cap_tokens':2400,'embedding_format':'raw-text-v1/no-application-prefix','oracle':'manually source-reviewed exact evidence; source-only full chunks vs compact situation clauses','judge':'same agent as annotation; not independent; conservative notes retained'},'retrieval':{},'generation':{}}
for variant,run in [('control','control-retrieval'),('A','A-branchsafe-retrieval'),('B','B-retrieval')]:
 rs=read(ROOT/'runs'/run/'results.json');metrics={}
 for mode in ['vector','hybrid']:
  xs=[r for r in rs if r['mode']==mode and cases[r['case']]['evidence']];hit1=0;rr=0
  for r in xs:
   es=cases[r['case']]['evidence'];sel=r['debug']['search']['selected'];rank=next((i for i,x in enumerate(sel,1) if any(match(e,x) for e in es)),None);hit1+=rank==1;rr+=0 if rank is None else 1/rank
  metrics[mode]={'hit_at_1_any_required_evidence':frac(hit1,len(xs)),'mrr_at_5':{'reciprocal_rank_sum':rr,'denominator':len(xs),'value':rr/len(xs)},'candidate_all_evidence':frac(sum(r['candidate_complete'] for r in xs),len(xs)),'context_all_evidence':frac(sum(r['context_complete'] for r in xs),len(xs)),'context_tokens':stats([r['debug']['context_tokens'] for r in xs]),'retrieval_ms_cached_query_embeddings':stats([r['debug']['stage_ms']['retrieval'] for r in xs])}
 summary['retrieval'][variant]=metrics
for name in ['control-local','A-branchsafe-local','B-local','control-oracle','B-oracle']:
 path=ROOT/'runs'/name;rs=read(path/'results.json');rv=read(path/'review.json')['records'];by={r['case']:r for r in rv};m={}
 for split in [None,'existing-regression','independent-source-diagnostic']:
  xs=[r for r in rs if split is None or cases[r['case']]['split']==split];v=[by[r['case']] for r in xs];ans=[r for r in v if cases[r['case']]['evidence']];unans=[r for r in v if not cases[r['case']]['evidence']]
  result={key:frac(sum(r[key] for r in v if key in r),sum(key in r for r in v)) for key in ['task_pass','strict_pass','unsupported_addition','spoiler']}
  result['episode_error_answerable']=frac(sum(r['episode_error'] for r in ans),len(ans));result['safe_unanswerable']=frac(sum(r['task_pass'] for r in unans),len(unans))
  for key in ['exact_code','exact_sequence']:result[key]=frac(sum(r[key] for r in v if key in r),sum(key in r for r in v))
  result['runtime_success']=frac(sum(not r.get('error') for r in xs),len(xs));m[split or 'all']=result
 m['context_tokens']=stats([r['debug']['context_tokens'] for r in rs]);m['generation_seconds_including_retry']=stats([r['debug']['stage_ms']['generation']/1000 for r in rs]);m['total_seconds_cached_embeddings']=stats([r['debug']['stage_ms']['total']/1000 for r in rs]);m['prompt_tokens_last_attempt']=stats([r['debug']['completion_attempts'][-1]['prompt_tokens'] for r in rs if r['debug']['completion_attempts']]);summary['generation'][name]=m
control=read(ROOT/'runs/control-local/review.json')['records'];b=read(ROOT/'runs/B-local/review.json')['records'];a=read(ROOT/'runs/A-branchsafe-local/review.json')['records']
for name,other in [('A',a),('B',b)]:
 cby={r['case']:r for r in control};summary['generation'][name+'-paired-vs-control']={'improved':[r['case'] for r in other if r['task_pass'] and not cby[r['case']]['task_pass']],'regressed':[r['case'] for r in other if not r['task_pass'] and cby[r['case']]['task_pass']]}
summary['database_integrity']=read(ROOT/'build-recovered-v3/integrity-report.json');summary['preparation']=read(ROOT/'build-recovered-v3/preparation.json')
summary['langfuse']={'items':0,'scores':0,'experiments':[],'project':'http://localhost:3000/project/walkthrough-backend'}
for name in ['control-retrieval','A-branchsafe-retrieval','B-retrieval','control-local','A-branchsafe-local','B-local','control-oracle','B-oracle']:
 files=list((ROOT/'runs'/name).glob('.langfuse-*.json'));assert len(files)==1
 state=read(files[0]);summary['langfuse']['items']+=state['records'];summary['langfuse']['scores']+=len(state['scores']);summary['langfuse']['experiments']+=state['experiments']
path=ROOT/'comparison.json'
if path.exists():assert read(path)==summary
else:path.write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
print('Retrieval:')
for v,x in summary['retrieval'].items():
 print(v,[(k,{z:y[z] for z in ['hit_at_1_any_required_evidence','mrr_at_5','candidate_all_evidence','context_all_evidence']}) for k,y in x.items()])
print('Generation:')
for name,m in summary['generation'].items():
 if 'all' in m:print(name,json.dumps(m,ensure_ascii=False))
print('Langfuse',summary['langfuse']['items'],summary['langfuse']['scores'],len(summary['langfuse']['experiments']))
