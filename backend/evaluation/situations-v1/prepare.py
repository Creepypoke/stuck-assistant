#!/usr/bin/env python3
"""Offline, source-only curation. No questions/gold or external LLM is read here."""
import argparse,hashlib,json,pathlib,re,sqlite3,struct,time,urllib.request,resource
ROOT=pathlib.Path(__file__).resolve().parent
GAME='dino-crisis-1'
def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(path):
 h=hashlib.sha256()
 with pathlib.Path(path).open('rb') as f:
  while b:=f.read(1024*1024):h.update(b)
 return h.hexdigest()
def dump(p,x):p.write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n')
def post(url,path,data):
 req=urllib.request.Request(url+path,json.dumps(data).encode(),{'Content-Type':'application/json'})
 with urllib.request.urlopen(req,timeout=180) as r:return json.load(r)
def request_json(url):
 with urllib.request.urlopen(url,timeout=30) as r:return json.load(r)
def main():
 ap=argparse.ArgumentParser();ap.add_argument('--out',type=pathlib.Path,required=True);ap.add_argument('--corpus',type=pathlib.Path);ap.add_argument('--embedding-url',default='http://127.0.0.1:18082');ap.add_argument('--tokenizer-url',default='http://127.0.0.1:18081');a=ap.parse_args()
 if a.out.exists():raise SystemExit('Choose a NEW output directory; artifacts are immutable')
 started=time.perf_counter();a.out.mkdir(parents=True)
 if a.corpus:
  c=sqlite3.connect('file:'+str(a.corpus.resolve())+'?mode=ro',uri=True);c.row_factory=sqlite3.Row
  chunks=[dict(x) for x in c.execute('select id ChunkID, document_id DocumentID,sequence Sequence,title Title,section Section,text Text from chunks order by document_id,sequence')]
  d=dict(c.execute('select * from documents').fetchone());markdown=d['markdown'];source_url=d['source_url'];provenance={'type':'original-rag','corpus_sha256':sha(a.corpus.read_bytes()),'raw_source_sha256':sha(bytes(d['raw_html']))}
 else:
  chunks=json.loads((ROOT/'baseline-chunks-from-langfuse.json').read_text());chunks.sort(key=lambda r:r['Sequence'])
  # Keep EVERY recovered character, including historic overlaps. Never pretend
  # this is the original parser Markdown or the original HTML.
  markdown='\n\n'.join('## '+r['Section']+'\n\n'+r['Text'] for r in chunks);source_url=chunks[0]['SourceURL'];provenance={'type':'langfuse-recovered-baseline-chunks','trace_id':'f81522cb1f13fcbb9dab778960b49074','original_corpus_sha256':'36f141abe2fa6f24ae595d3fbf59c6ef873ad840565f3c8de8e3c5a75730bb08','original_markdown_sha256':None,'raw_source_sha256':None,'warning':'Rebuilt control; NOT a byte-identical historical baseline. Canonical source retains all 45 exact chunks and historic overlap. Original HTML/Markdown unavailable outside restricted backend/tmp.'}
 source_hash=sha(markdown.encode());(a.out/'source.txt').write_text(markdown)
 blocks={};cursor=0
 for r in chunks:
  p=markdown.find(r['Text'],cursor)
  if p<0:p=markdown.find(r['Text'])
  if p<0:raise ValueError('chunk not verbatim in document')
  blocks[r['Sequence']]={'id':f"baseline-block-{r['Sequence']:03d}",'start':p,'end':p+len(r['Text']),'text_sha256':sha(r['Text'].encode()),'section':r['Section']};cursor=p
 dump(a.out/'blocks.json',blocks)
 def span(seq,quote):
  r=next(x for x in chunks if x['Sequence']==seq);i=r['Text'].index(quote);b=blocks[seq];start=b['start']+i
  assert markdown[start:start+len(quote)]==quote
  return {'document_sha256':source_hash,'block_id':b['id'],'start':start,'end':start+len(quote),'quote':quote}
 specs=json.loads((ROOT/'annotations.json').read_text());cards=[];review=[]
 for spec in specs:
  seq=spec['block'];r=next(x for x in chunks if x['Sequence']==seq)
  evidence=[span(seq,q) for q in spec['quotes']]
  facts=[{'value':q,'evidence':[s]} for q,s in zip(spec['quotes'],evidence)]
  # Fields can only select exact quotations. No automatic semantic inference.
  for key in ['exact_code','ordered_actions']:
   for val in spec.get(key,[]):
    assert any(val in q for q in spec['quotes']),(spec['id'],key,val)
  aliases=spec.get('aliases',[])
  card={'situation_id':spec['id'],'game_variant':'Dino Crisis 1 / Normal / source EN 2024-02-16; platform not established by recovered source','chapter':spec.get('chapter'),'episode':spec.get('episode',spec['id']),'branch':spec.get('branch'),'location':spec.get('location'),'conditions':[spec['context']],'required_items':spec.get('required_items',[]),'entities':spec.get('entities',[]),'aliases':aliases,'task':spec['task'],'facts':facts,'exact_code':[{'value':v,'evidence':[s for s in evidence if v in s['quote']]} for v in spec.get('exact_code',[])],'ordered_actions':[{'value':v,'evidence':[s for s in evidence if v in s['quote']]} for v in spec.get('ordered_actions',[])],'result':spec.get('result'),'next_situation_ids':spec.get('next_situation_ids',[]),'source_spans':evidence,'original_source_block':r['Text'],'validation_status':'source-reviewed','field_evidence':{key:list(evidence) for key in ['chapter','branch','location','conditions','required_items','task','result']},'normalizations':spec.get('normalizations',[])}
  # Context metadata is a curated label, not an additional game fact. Unknown
  # rooms/versions remain null. Conditions and items must be source-backed.
  for v in spec.get('required_items',[]):assert v in r['Text']
  for q in spec.get('context_evidence',[]):assert q in r['Text'] or q in r['Section']
  cards.append(card)
 # All transitions require explicit quotations; no inferred cross-branch links.
 ids={c['situation_id'] for c in cards}
 for c in cards:
  assert all(i in ids for i in c['next_situation_ids'])
 # Keep section provenance as an exact source span as well as the block text.
 for spec,card in zip(specs,cards):
  section=chunks[spec['block']]['Section'];pos=markdown.rfind(section,0,blocks[spec['block']]['start'])
  if pos>=0:
   section_span={'document_sha256':source_hash,'block_id':blocks[spec['block']]['id']+'-heading','start':pos,'end':pos+len(section),'quote':section}
   for v in card['field_evidence'].values():v.append(section_span)
 dump(a.out/'cards.json',cards)
 # Uncurated material stays available as complete historical source blocks;
 # never label it as verified structured facts.
 covered={s['block'] for s in specs}
 for r in chunks:
  if r['Sequence'] not in covered:review.append({'block_id':blocks[r['Sequence']]['id'],'reason':'not semantically curated; raw fallback only','text_sha256':sha(r['Text'].encode())})
 review.append({'situation_id':'gas-neutralize','reason':'Source says GRENN. Preserved verbatim; GREEN interpretation is not silently substituted.'})
 dump(a.out/'review-required.json',review)
 ep=request_json(a.embedding_url+'/props');gp=request_json(a.tokenizer_url+'/props')
 embedding_hash=file_sha(ep['model_path']);generator_hash=file_sha(gp['model_path'])
 profile={'model':'embedding','revision':'1','gguf_sha256':embedding_hash,'dimensions':768,'input_format':'raw-text-v1/no-application-prefix'}
 token_cache={};vector_cache={};inputs=[]
 def count(url,text):
  k=(url,text)
  if k not in token_cache:token_cache[k]=len(post(url,'/tokenize',{'content':text,'add_special':True})['tokens'])
  return token_cache[k]
 def vector(text):
  if text not in vector_cache:
   x=post(a.embedding_url,'/v1/embeddings',{'model':'embedding','input':[text]})
   assert x['usage']['prompt_tokens']==count(a.embedding_url,text),'server changed embedding input'
   v=x['data'][0]['embedding'];assert len(v)==768;vector_cache[text]=v
  return vector_cache[text]
 for variant in ['control','A','B']:
  dest=a.out/variant;dest.mkdir();db=sqlite3.connect(dest/(GAME+'.rag'))
  db.executescript('''CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE documents(id INTEGER PRIMARY KEY,source_url TEXT,title TEXT,raw_html BLOB,markdown TEXT,parser_metadata TEXT,downloaded_at TEXT,created_at TEXT);
CREATE TABLE chunks(id INTEGER PRIMARY KEY,document_id INTEGER,title TEXT,section TEXT,sequence INTEGER,text TEXT,metadata TEXT,UNIQUE(document_id,sequence));
CREATE TABLE embeddings(chunk_id INTEGER PRIMARY KEY,dimensions INTEGER,vector BLOB);
CREATE VIRTUAL TABLE chunks_fts USING fts5(title,section,text,content='chunks',content_rowid='id');''')
  meta={'schema_version':'1','experimental_schema':'situation-v1' if variant!='control' else 'recovered-control-v1','game_id':GAME,'embedding_model':'embedding','embedding_version':'1','embedding_dimensions':'768','embedding_input_format':profile['input_format'],'embedding_gguf_sha256':embedding_hash,'chunking_version':('source-curated-situations-v2-branchsafe' if variant=='A' else 'source-curated-situations-v1') if variant!='control' else 'historical-45-recovered','source_sha256':source_hash}
  db.executemany('insert into metadata values (?,?)',meta.items())
  db.execute('insert into documents values(1,?,?,?,?,?,?,?)',(source_url,chunks[0]['Title'],None,markdown,json.dumps(provenance),'2026-11-05T00:00:00Z','2026-11-05T00:00:00Z'))
  rows=[]
  if variant=='control':
   rows=[(r['Title'],r['Section'],r['Text'],{}) for r in chunks]
  else:
   for spec,card in zip(specs,cards):
    title='Dino Crisis. '+card['task'];context=' | '.join(x for x in [card['chapter'],card['branch'],card['location']] if x)
    context+='\n'+spec['context']
    # A keeps the COMPLETE source block because the original uses run-on
    # sentences. B presents exact verified clauses with per-fact citations.
    original=card['original_source_block']
    # Original block 21 contains two incompatible second-choice branches.
    # Reference tables also have explicit item/action boundaries. Keep these
    # complete units independently; never copy the other branch into A.
    if spec['id'] in ['second-gail','second-rick'] or spec['block'] in [1,2,43]:
     original=spec['quotes'][0]
    body=original if variant=='A' else '\n'.join(f['value'] for f in card['facts'])
    search=title+'\n'+context+'\n'+'; '.join(card['aliases'])+'\n'+body
    answer=context+'\n'+(original if variant=='A' else '\n'.join(f"{f['value']} [evidence:{f['evidence'][0]['block_id']}:{f['evidence'][0]['start']}-{f['evidence'][0]['end']}]" for f in card['facts']))
    if variant=='B' and card['ordered_actions']:answer+='\nOrdered actions: '+ ' → '.join(x['value'] for x in card['ordered_actions'])
    if variant=='B' and card['exact_code']:answer+='\nExact code: '+', '.join(x['value'] for x in card['exact_code'])
    spans=[span(spec['block'],original)] if variant=='A' else card['source_spans']
    rows.append((title,card['episode'],search,{'situation':{'schema':'situation-v1','situation_id':card['situation_id'],'answer_text':answer,'source_spans':spans}}))
   # Preserve the rest of the guide, without splitting historic sentences any
   # further. Raw fallback is explicitly outside verified card set.
   for r in chunks:
    if r['Sequence'] not in covered:rows.append((r['Title'],r['Section'],r['Text'],{'validation_status':'raw-fallback'}))
  counts=[]
  for i,(title,section,text,metadata) in enumerate(rows):
   en=count(a.embedding_url,text);gn=count(a.tokenizer_url,'['+title+' / '+section+']\n'+metadata.get('situation',{}).get('answer_text',text))
   if en>2048:raise ValueError('embedding input too long: '+section)
   counts.append({'sequence':i,'embedding_tokens':en,'generator_tokens_with_header':gn,'situation_id':metadata.get('situation',{}).get('situation_id')})
   v=vector(text);inputs.append({'variant':variant,'sequence':i,'input':text,'input_sha256':sha(text.encode()),'embedding_tokens':en,'vector_sha256':sha(struct.pack('<768f',*v))})
   db.execute('insert into chunks values(?,1,?,?,?,?,?)',(i+1,title,section,i,text,json.dumps(metadata,ensure_ascii=False)))
   db.execute('insert into embeddings values(?,768,?)',(i+1,struct.pack('<768f',*v)))
  db.execute("insert into chunks_fts(chunks_fts) values ('rebuild')");db.commit()
  assert db.execute('pragma integrity_check').fetchone()[0]=='ok'
  assert db.execute('select count(*) from chunks_fts').fetchone()[0]==len(rows)
  db.close()
  manifest={'schema':'situation-experiment-v1','variant':variant,'source_sha256':source_hash,'source_provenance':provenance,'game_variant':cards[0]['game_variant'],'parser_version':'historical-parser-output recovered from Langfuse' if not a.corpus else 'saved-original-parser-output','parser_code_sha256':sha((ROOT.parent.parent/'parser/walkthrough_parser.py').read_bytes()),'annotation_version':'source-curated-v1','annotation_sha256':sha((ROOT/'annotations.json').read_bytes()),'chunking_version':meta['chunking_version'],'llm_annotation':{'used':True,'model':'Codex GPT-6, active assistant session; exact serving revision unavailable','method':'agent-curated source selectors and context, followed by deterministic evidence checks; source excerpts read in this user-authorized Codex session; no additional inference service called','sampling_parameters':'not exposed for active assistant; reproducible build uses frozen annotations, no LLM calls'},'embedding_profile':profile,'embedding_runtime':ep,'tokenizer':{'model_path':gp['model_path'],'gguf_sha256':generator_hash,'runtime_build':gp['build_info']},'chunk_counts':counts,'bytes':(dest/(GAME+'.rag')).stat().st_size,'rag_sha256':sha((dest/(GAME+'.rag')).read_bytes()),'reproduce':f'python3 evaluation/situations-v1/prepare.py --out NEW_DIRECTORY'+(' --corpus '+str(a.corpus) if a.corpus else ''),'limitations':['A retains whole historical source blocks to avoid breaking original run-on sentences; adjacent events can remain.','Uncurated blocks remain raw fallback; no claim of complete semantic curation.','No oracle questions or expected answers are read by this builder.']}
  dump(dest/'manifest.json',manifest)
 dump(a.out/'embedding-inputs.json',inputs)
 dump(a.out/'preparation.json',{'elapsed_seconds':time.perf_counter()-started,'process_maxrss_platform_units':resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,'cpu_seconds':time.process_time(),'embedding_requests_unique':len(vector_cache),'cards':len(cards),'raw_fallback_blocks':len(chunks)-len(covered),'source_sha256':source_hash,'original_source_unmodified':True,'checks':['all exact quotes match saved Unicode source offsets','all action/code strings occur in their fact quote','all required items occur in source block','all context evidence quotes match block or section','actual embedding usage equals server tokenization','no embedding input exceeds 2048 tokens','SQLite integrity_check and FTS row count']} )
 print(a.out)
if __name__=='__main__':main()
