#!/usr/bin/env python3
"""Source-reviewed, exact-result-bound manual scores; never grade a new sample automatically."""
import hashlib,importlib.util,json,pathlib,re
HERE=pathlib.Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('previous_review_definitions',HERE.parent/'review.py');previous=importlib.util.module_from_spec(spec);spec.loader.exec_module(previous)
EXPECTED_RESULTS_SHA256 = {'control-local': '3369bb1ef977d581c4a0fd9017bd51e5c9c0f68a5aa19216498550e5800dd1bb', 'A-local': '48e809516a8522ada846d10359f255709a59f00e1e97fef1f3228d60e0e7a9a3', 'B-local': 'fa09470621a8691fa3e4889b3dade7aa0535ee3c368115b0c3c187ed273bcb9a', 'control-oracle': 'c6ff6ea177e11f167b8b4fe52cc6478b589cb4db8c3ecba32aae5c5d7c2fde53', 'B-oracle': '53faddde5eb9d4a0db204790126a1734083a53846b371b2b8790b015c41ba4b7'}

def main():
 data=json.loads((HERE/'judgments.json').read_text())
 assert set(data)==set(EXPECTED_RESULTS_SHA256)
 for name,s in data.items():
  path=HERE/'runs'/name;raw=(path/'results.json').read_bytes();assert hashlib.sha256(raw).hexdigest()==EXPECTED_RESULTS_SHA256[name],'New answers need new source review'
  rs=json.loads(raw);passed=set(s['pass'].split());failed=s['fail'];assert passed.isdisjoint(failed);assert passed|set(failed)=={r['case'] for r in rs},name
  records=[]
  for r in rs:
   id=r['case'];entry={k:r[k] for k in ['case','mode','profile','context_mode','answer','expected','context_sha256','prompt_sha256']}
   task=id in passed
   entry.update(task_pass=task,strict_pass=task and id not in s['labels'].split(),episode_error=id in s['episode'].split(),unsupported_addition=id in s['unsupported'].split(),spoiler=id in s['spoiler'].split(),review_note=failed.get(id,'Requested facts/conditions checked against frozen source. Supported non-spoiler orientation and unambiguous translations accepted as in 2B rubric.'))
   if id in previous.CODES:entry['exact_code']=all(re.search(r'(?<!\d)'+code+r'(?!\d)',r['answer']) for code in previous.CODES[id])
   if id in previous.SEQUENCES:entry['exact_sequence']=id not in s['sequence_wrong'].split()
   if name=='A-local' and id=='ambiguous-computer':entry['review_note']='Two correct conditional chapter-2 examples followed by clarification; no arbitrary single password selected. Accepted under existing rubric; a policy banning all code examples would be a different experiment.'
   if name=='A-local' and id=='red-box':entry['review_note']='Two Plug/ammunition correct. Ordinary optional box/map actions match source; no false requirement or new narrative outcome is asserted.'
   if name=='A-local' and id=='unanswerable-battery-code':entry['review_note']='Rejects numeric-code premise by explaining directional options and gives full source sequence; does not invent numeric code.'
   records.append(entry)
  out={'rubric':previous.RUBRIC,'reviewer':'Codex GPT-6; source review by same agent as prior 2B experiment, not independent or blinded','sequence_rule':'Same as 2B: required operations/order/repeats, RU direction/color mapping allowed, no silent GRENN normalization. Correct codes/sequences can coexist with failed full answers.','records':records}
  dest=path/'review.json'
  if dest.exists():assert json.loads(dest.read_text())==out
  else:dest.write_text(json.dumps(out,ensure_ascii=False,indent=2)+'\n')
  print(name,'task',sum(r['task_pass'] for r in records),'strict',sum(r['strict_pass'] for r in records),'/',len(records))
if __name__=='__main__':main()
