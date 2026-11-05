#!/usr/bin/env python3
"""Independent artifact checks. Does not modify any corpus or historical run."""
import pathlib,json,hashlib,sqlite3,struct,argparse
p=argparse.ArgumentParser();p.add_argument('--build',type=pathlib.Path,required=True);a=p.parse_args();root=pathlib.Path(__file__).resolve().parent
sha=lambda b:hashlib.sha256(b).hexdigest()
source=(a.build/'source.txt').read_text();cards=json.loads((a.build/'cards.json').read_text());blocks=json.loads((a.build/'blocks.json').read_text());old=json.loads((root.parent/'dino-crisis-fts/questions.json').read_text());recovered={r['Sequence']:r for r in json.loads((root/'baseline-chunks-from-langfuse.json').read_text())}
old_checked=0
for c in old:
 for e in c.get('evidence',[]):assert sha(recovered[e['sequence']]['Text'].encode())==e['text_sha256'];old_checked+=1
span_count=0;actions=0;codes=0
for c in cards:
 assert c['validation_status']=='source-reviewed'
 for f in c['facts']+c['exact_code']+c['ordered_actions']:
  assert f['evidence'];assert isinstance(f['value'],str)
  for s in f['evidence']:
   assert s['document_sha256']==sha(source.encode());assert source[s['start']:s['end']]==s['quote'];assert s['block_id'] in {b['id'] for b in blocks.values()};assert f['value'] in s['quote'];span_count+=1
  assert all(s['block_id'] in {x['block_id'] for x in c['source_spans']} for s in f['evidence'])
 actions+=len(c['ordered_actions']);codes+=len(c['exact_code'])
# Source-aware checks of the discriminating facts and branch applicability.
by={c['situation_id']:c for c in cards}
assert [x['value'] for x in by['generator-first']['ordered_actions']]==['CENTER','RIGHT','CENTER','CANCEL','pull the levers']
assert [x['value'] for x in by['generator-second']['ordered_actions']]==['LEFT','CENTER','RIGHT','LEFT','CENTER','LEFT','CANCEL','pull the levers']
assert [x['value'] for x in by['gas-neutralize']['ordered_actions']]==['BLUE','GREEN','GRENN','RED','BLUE']
for id,code in [('b1-chip-panel','3695'),('b2-chips-panel','0392'),('safe-office-early','0426'),('office-id-early','58104'),('third-gail-id','31415'),('third-rick-id','31415'),('third-rick-0367','0367'),('third-rick-0204','0204')]:assert by[id]['exact_code'][0]['value']==code
for id,branch in [('first-gail-immediate','first-choice/Gail'),('first-rick-immediate','first-choice/Rick'),('second-gail','second-choice/Gail'),('second-rick','second-choice/Rick'),('third-gail-id','third-choice/Gail'),('third-rick-id','third-choice/Rick')]:assert by[id]['branch']==branch
inputs=json.loads((a.build/'embedding-inputs.json').read_text());results={}
for v in ['control','A','B']:
 path=a.build/v/'dino-crisis-1.rag';initial=sha(path.read_bytes());m=json.loads((a.build/v/'manifest.json').read_text());assert initial==m['rag_sha256']
 db=sqlite3.connect('file:'+str(path.resolve())+'?mode=ro',uri=True)
 assert db.execute('pragma integrity_check').fetchone()[0]=='ok'
 rows=list(db.execute('select c.sequence,c.text,c.metadata,e.vector from chunks c join embeddings e on e.chunk_id=c.id'))
 view_count=0
 for seq,text,metadata,vector in rows:
  item=next(i for i in inputs if i['variant']==v and i['sequence']==seq)
  assert item['input']==text;assert item['input_sha256']==sha(text.encode());assert item['vector_sha256']==sha(vector)
  assert len(vector)==768*4
  s=json.loads(metadata).get('situation')
  if s:
   view_count+=1
   for sp in s['source_spans']:assert sp['quote'] in text and sp['quote'] in s['answer_text'];assert source[sp['start']:sp['end']]==sp['quote']
   if v=='B':assert s['answer_text']!=text
 if v!='control':
  assert db.execute("select count(*) from chunks_fts where chunks_fts match 'цифровой'").fetchone()[0]>0
 assert db.execute('select count(*) from chunks_fts').fetchone()[0]==len(rows)
 db.close();assert sha(path.read_bytes())==initial
 results[v]={'chunks':len(rows),'verified_situation_views':view_count,'bytes':path.stat().st_size,'rag_sha256':initial,'max_embedding_tokens':max(x['embedding_tokens'] for x in m['chunk_counts']),'max_generator_tokens_with_header':max(x['generator_tokens_with_header'] for x in m['chunk_counts'])}
result={'original_regression_chunk_hashes_verified':f'{old_checked}/{old_checked}','recovered_chunks':len(recovered),'cards':len(cards),'fact_action_code_evidence_links_checked':span_count,'ordered_actions_including_repeats':actions,'exact_code_strings':codes,'source_sha256':sha(source.encode()),'sqlite_and_embedding_input_checks':results,'semantic_checks':'Agent source review plus explicit branch/code/repetition assertions; not an independent human review','limitations':['Original HTML/Markdown and original .rag byte identity not verified.','39 curated situations; 20 uncurated raw fallback blocks.','A retains old whole blocks including historic splits/overlap; full sentence-safe resegmentation requires original source.']}
out=a.build/'integrity-report.json'
if out.exists():assert json.loads(out.read_text())==result
else:out.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(result,ensure_ascii=False,indent=2))
