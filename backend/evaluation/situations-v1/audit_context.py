#!/usr/bin/env python3
"""Attach explicit source evidence to context fields and entity translations.
This companion is authoritative card metadata; .rag runtime views stay frozen.
"""
import argparse,json,pathlib,hashlib
p=argparse.ArgumentParser();p.add_argument('--build',type=pathlib.Path,required=True);a=p.parse_args()
src=(a.build/'source.txt').read_text();h=hashlib.sha256(src.encode()).hexdigest();cards=json.loads((a.build/'cards.json').read_text());blocks=json.loads((a.build/'blocks.json').read_text());byblock={b['id']:b for b in blocks.values()}
names={'Gail':'Гейл','Rick':'Рик','Regina':'Регина','Tom':'Том','Dr. Kirk':'доктор Кирк','Control Room':'комната управления','Shotgun':'дробовик','Handgun':'пистолет','Emergency Box':'аварийный ящик','B1 Key Chip':'чип B1','B2 Key Chip':'чип B2','Planning Disc':'планирующий диск','ID Card':'идентификационная карта','Initializer':None,'Stabilizer':None,'Med Pak M':None,'Med Pak L':None,'Resuscitation':None,'Entrance Key':None,'Startup Battery R':None,'Small Size Key':None,'Plug':None,'Hemostat':'гемостат'}
checked=0
for c in cards:
 b=byblock[c['source_spans'][0]['block_id']];original=c['original_source_block'];assert src[b['start']:b['end']]==original
 whole={'document_sha256':h,'block_id':b['id'],'start':b['start'],'end':b['end'],'quote':original}
 # Cite the complete source block for state assertions that may precede the
 # focused action (e.g. returning with BG Room B1 Key). The generator does not
 # receive this broad evidence quote; it remains an auditable reference.
 for field in c['field_evidence']:
  c['field_evidence'][field]=[whole]+[s for s in c['field_evidence'][field] if s['block_id'].endswith('-heading')]
 # Second choice is a bold heading INSIDE a historical block, not Section.
 if c['situation_id'] in ('second-gail','second-rick'):
  marker='**SECOND CHOICE – OPTION '+('1 – FOLLOWING GAIL**' if c['situation_id']=='second-gail' else '2 – FOLLOWING RICK’S IDEA**')
  assert marker in original
  i=b['start']+original.index(marker);ref={'document_sha256':h,'block_id':b['id'],'start':i,'end':i+len(marker),'quote':marker}
  c['field_evidence']['branch']=[ref]+c['source_spans']
 c['entities']=[]
 for en,ru in names.items():
  if en not in original:continue
  i=b['start']+original.index(en);ref={'document_sha256':h,'block_id':b['id'],'start':i,'end':i+len(en),'quote':en}
  c['entities'].append({'source_name':en,'verified_aliases':[] if ru is None else [{'value':ru,'kind':'agent-reviewed RU translation/transliteration; not a new game label','evidence':[ref]}],'evidence':[ref]})
  # Normal difficulty and source date are asserted by the recovered preface.
 preface=blocks['0'];c['field_evidence']['game_variant']=[{'document_sha256':h,'block_id':preface['id'],'start':preface['start'],'end':preface['end'],'quote':src[preface['start']:preface['end']]}]
 if c['situation_id']=='b2-chips-panel':
  c['required_items']=['B2 Key Chip 1','B2 Key Chip 2']
  c['field_evidence']['required_items']=[]
  for seq,item in [(26,'B2 Key Chip 1'),(27,'B2 Key Chip 2')]:
   ib=blocks[str(seq)];i=src.index(item,ib['start'],ib['end'])
   c['field_evidence']['required_items'].append({'document_sha256':h,'block_id':ib['id'],'start':i,'end':i+len(item),'quote':item})
 for refs in c['field_evidence'].values():
  for s in refs:assert s['document_sha256']==h and src[s['start']:s['end']]==s['quote'];checked+=1
 c['context_validation_status']='agent-reviewed against full original block and explicit branch headings; no independent human review'
result={'schema':'situation-v1-context-audit-v2','source_sha256':h,'field_evidence_links_checked':checked,'cards':cards}
out=a.build/'cards-reviewed-v2.json'
if out.exists():assert json.loads(out.read_text())==result
else:out.write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(len(cards),'cards;',checked,'context evidence links verified')
