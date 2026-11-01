from __future__ import annotations
import concurrent.futures as cf,collections,csv,datetime,gzip,hashlib,io,json,os,pathlib,posixpath,re,subprocess,tarfile,tempfile,threading,time,urllib.request
ROOT=pathlib.Path(__file__).resolve().parent
SNAP=ROOT/'snapshots'; CACHE=pathlib.Path(os.environ.get('GCODE_AUDIT_CACHE') or tempfile.mkdtemp(prefix='gcode-market-audit-'))
UA='GCode-public-marketplace-static-audit/1.0'
MAX_COMPRESSED=25*1024*1024
lock=threading.Lock(); archive_futures={}; resolve_futures={}; pool=cf.ThreadPoolExecutor(max_workers=5)
PRIOR={}
if os.environ.get('GCODE_AUDIT_OFFLINE') and (ROOT/'marketplace-audit.json').exists():
 for _r in json.loads((ROOT/'marketplace-audit.json').read_text('utf-8'))['plugins']:
  if _r.get('source_commit') and _r.get('scan_depth')!='catalog_only':
   _s=_r.get('source');PRIOR[(_r.get('source_repo'),_s.get('ref') if isinstance(_s,dict) else None)]=_r['source_commit']

def request(url):
 return urllib.request.urlopen(urllib.request.Request(url,headers={'User-Agent':UA}),timeout=35)

def repo_name(url):
 if not isinstance(url,str):return None
 if re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+',url):return url.removesuffix('.git')
 m=re.match(r'(?:https?://github.com/|git@github.com:)([^/]+/[^/#?]+)',url)
 return m.group(1).removesuffix('.git') if m else None

def resolve(repo,ref=None,sha=None):
 if sha and re.fullmatch('[0-9a-fA-F]{40}',sha):return sha.lower()
 if (repo,ref) in PRIOR:return PRIOR[(repo,ref)]
 if os.environ.get('GCODE_AUDIT_OFFLINE'):raise RuntimeError('offline_ref_not_previously_resolved')
 k=(repo,ref)
 with lock:
  fut=resolve_futures.get(k)
  if fut is None:
   fut=cf.Future();resolve_futures[k]=fut;owner=True
  else:owner=False
 if not owner:return fut.result()
 try:
  out=subprocess.run(['git','ls-remote','https://github.com/'+repo+'.git',*( [ref,'refs/heads/'+ref,'refs/tags/'+ref,'refs/tags/'+ref+'^{}'] if ref else ['HEAD'])],capture_output=True,text=True,timeout=40)
  if out.returncode:raise RuntimeError('git ls-remote failed: '+out.stderr.strip()[:300])
  lines=[x.split() for x in out.stdout.splitlines() if x.strip()]
  if not lines:raise RuntimeError('source ref did not resolve: '+str(ref))
  matches=[x[0] for x in lines if x[1].endswith('^{}')]
  result=(matches or [x[0] for x in lines])[0];fut.set_result(result);return result
 except Exception as e:fut.set_exception(e);raise

def archive(repo,sha):
 k=(repo,sha)
 with lock:
  fut=archive_futures.get(k)
  if fut is None:fut=cf.Future();archive_futures[k]=fut;owner=True
  else:owner=False
 if not owner:return fut.result()
 try:
  url='https://codeload.github.com/'+repo+'/tar.gz/'+sha
  cachepath=CACHE/(repo.replace('/','--')+'--'+sha+'.tar.gz')
  if cachepath.exists():data=cachepath.read_bytes()
  else:
   if os.environ.get('GCODE_AUDIT_OFFLINE'):raise RuntimeError('archive_not_cached_after_prior_size_limit')
   with request(url) as r:
    if int(r.headers.get('Content-Length','0'))>MAX_COMPRESSED:raise RuntimeError('archive exceeds 25 MiB limit')
    data=r.read(MAX_COMPRESSED+1)
  if len(data)>MAX_COMPRESSED:raise RuntimeError('archive exceeds 25 MiB limit')
  (CACHE/(repo.replace('/','--')+'--'+sha+'.tar.gz')).write_bytes(data)
  files={}; all_paths=[]; total=0
  with tarfile.open(fileobj=io.BytesIO(data),mode='r:gz') as t:
   for m in t:
    if not m.isfile():continue
    p=m.name.split('/',1)[1] if '/' in m.name else ''
    all_paths.append(p)
    if m.size>512*1024:continue
    if pathlib.PurePosixPath(p).suffix.lower() not in ('.md','.json','.yaml','.yml','.sh','.py','.js','.ts','.toml','.ps1','.bash') and pathlib.PurePosixPath(p).name not in ('LICENSE','Dockerfile','Makefile'):continue
    total+=m.size
    if total>32*1024*1024:continue
    f=t.extractfile(m)
    if f:
     try:files[p]=f.read().decode('utf-8-sig')
     except UnicodeDecodeError:pass
  result={'files':files,'paths':all_paths,'download_bytes':len(data),'url':url,'sha256':hashlib.sha256(data).hexdigest()}
  fut.set_result(result);return result
 except Exception as e:fut.set_exception(e);raise

def jparse(text,path,errors):
 try:return json.loads(text)
 except Exception as e:errors.append('invalid_json:'+path+':'+str(e));return None

def strings(x):
 if isinstance(x,str):yield x
 elif isinstance(x,dict):
  for v in x.values():yield from strings(v)
 elif isinstance(x,list):
  for v in x:yield from strings(v)

def audit(entry,market,catalog_only=False):
 out={'marketplace':market['name'],'marketplace_repo':market['repo'],'marketplace_commit':market['commit'],'name':entry.get('name'),'description':entry.get('description'),'source':entry.get('source'),'marketplace_entry':entry,'source_repo':None,'source_commit':None,'source_path':None,'source_access':'not_checked','scan_depth':'catalog_only' if catalog_only else 'source_tree','static_status':'catalog_only','runtime':'not_tested','manifest_status':'not_checked','component_counts':{},'components':{},'required_env':[],'dependency_hints':[],'feature_hints':[],'errors':[],'risks':[],'evidence':[]}
 try:
  src=entry.get('source');ref=None;sha=None;prefix=''
  if isinstance(src,str):
   if not src.startswith('./'):out['errors'].append('relative_source_missing_dot_slash')
   repo=market['repo'];sha=market['commit'];prefix=posixpath.normpath(src).strip('.') .strip('/')
  elif isinstance(src,dict):
   repo=repo_name(src.get('url') or ('https://github.com/'+src.get('repo','')));ref=src.get('ref');sha=src.get('sha');prefix=src.get('path','').strip('/')
   if src.get('path') and src.get('source')=='url':out['risks'].append('url_source_includes_path_requires_host_support_verification')
   if src.get('source') not in ('url','github','git-subdir'):out['risks'].append('source_type_not_audited:'+str(src.get('source')))
  else:raise ValueError('missing_or_invalid_source')
  out['source_repo']=repo;out['source_path']=prefix or '.'
  if not repo:raise ValueError('not_a_supported_github_source')
  if prefix.startswith('..') or '/..' in prefix:raise ValueError('source_path_traversal')
  if catalog_only:
   out['source_commit']=sha;out['risks'].append('source_tree_not_fetched_or_checked');return out
  resolved=resolve(repo,ref,sha);out['source_commit']=resolved
  ar=archive(repo,resolved);out['source_access']='archive_accessible'
  pref=prefix+'/' if prefix else ''
  paths=[p[len(pref):] for p in ar['paths'] if p.startswith(pref)]
  files={p[len(pref):]:s for p,s in ar['files'].items() if p.startswith(pref)}
  if not paths:out['errors'].append('source_subdirectory_empty_or_missing')
  manifest_path='.claude-plugin/plugin.json';manifest={}
  if manifest_path in files:
   manifest=jparse(files[manifest_path],manifest_path,out['errors']) or {};out['manifest_status']='present'
   if manifest.get('name') and manifest['name']!=entry.get('name'):out['risks'].append('marketplace_and_manifest_names_differ:'+str(manifest.get('name')))
  elif entry.get('strict') is False:out['manifest_status']='optional_inline_marketplace_definition'
  else:out['manifest_status']='absent';out['risks'].append('manifest_absent_auto_discovery_or_marketplace_metadata_must_be_supported')
  # Marketplace inline definitions are valid; strict affects merge/manifest policy, not recognition.
  effective={**manifest,**{k:v for k,v in entry.items() if k in ('skills','commands','agents','hooks','mcpServers','lspServers','settings')}}
  comp={k:[] for k in ['skills','commands','agents','hooks','mcp','lsp','settings']}
  comp['skills']=[p for p in paths if p.startswith('skills/') and p.endswith('/SKILL.md')]
  if 'skills' in effective:
   skillrefs=effective['skills']; skillrefs=skillrefs if isinstance(skillrefs,list) else [skillrefs]
   for s in skillrefs:
    if not isinstance(s,str):out['risks'].append('non_string_skills_path');continue
    target=s.removeprefix('./').rstrip('/')
    found=[p for p in paths if p==target+'/SKILL.md' or p.startswith(target+'/') and p.endswith('/SKILL.md')]
    comp['skills']+=found
    if not found:out['errors'].append('declared_skill_path_missing:'+s)
  else:comp['skills']=[p for p in paths if p.startswith('skills/') and p.endswith('/SKILL.md')]
  for key in ('commands','agents'):
   comp[key]=[] if key in effective else [p for p in paths if p.startswith(key+'/') and p.endswith('.md')]
   if key in effective:
    refs=effective[key];refs=refs if isinstance(refs,list) else [refs]
    for s in refs:
     if isinstance(s,str):
      s=s.removeprefix('./').rstrip('/');found=[p for p in paths if p==s or p.startswith(s+'/') and p.endswith('.md')]
      if not found:out['errors'].append('declared_'+key+'_path_missing:'+s)
      comp[key]+=found
     elif isinstance(s,dict):comp[key].append('inline:'+key)
  hookdata=[];mcpdata=[];lspdata=[]
  for key,default,ck,collector in [('hooks','hooks/hooks.json','hooks',hookdata),('mcpServers','.mcp.json','mcp',mcpdata),('lspServers','.lsp.json','lsp',lspdata)]:
   refs=[]
   if default in paths:refs.append(default)
   value=effective.get(key)
   if isinstance(value,dict):comp[ck].append('inline:'+key);collector.append(value)
   elif isinstance(value,str):refs.append(value.removeprefix('./'))
   elif isinstance(value,list):
    for x in value:
     if isinstance(x,str):refs.append(x.removeprefix('./'))
     elif isinstance(x,dict):comp[ck].append('inline:'+key);collector.append(x)
   for rp in dict.fromkeys(refs):
    if rp not in files:out['errors'].append('declared_component_file_missing_or_over_limit:'+rp);continue
    comp[ck].append(rp);d=jparse(files[rp],rp,out['errors'])
    if d is not None:collector.append(d)
  if 'settings' in effective:comp['settings'].append('inline:settings')
  if 'settings.json' in files:comp['settings'].append('settings.json')
  for key in comp:comp[key]=sorted(set(comp[key]))
  out['components']=comp;out['component_counts']={k:len(v) for k,v in comp.items()}
  alltext='\n'.join(files.values())
  env=sorted(set(re.findall(r'\$\{?((?:CLAUDE|ANTHROPIC|OPENAI|[A-Z0-9]+_API|[A-Z0-9]+_TOKEN|[A-Z0-9]+_SECRET|[A-Z0-9]+_KEY)[A-Z0-9_]*)',alltext)))
  out['required_env']=env
  deps=[]
  for dep,pat in {'claude_cli':r'(?m)(?:^|[\s`";|&])claude\s+(?:-p|--|plugin|mcp|auth|code)', 'bash':r'\bbash\b|#!/bin/(?:ba)?sh', 'jq':r'\bjq\b', 'python':r'\bpython[23]?\b','node':r'\b(?:node|npx|npm)\b','uv':r'\b(?:uvx|uv run)\b','git':r'\bgit\s+(?:clone|worktree|diff|status|log|show|rev-parse)','gh':r'\bgh\s+(?:pr|issue|api|auth|repo)','docker':r'\bdocker\b','browser':r'\b(?:Chrome|Chromium|Playwright|Puppeteer)\b','network_credentials':r'(?i)api[ -]?key|oauth|bearer|access[ -]?token','paid_service':r'(?i)paid|subscription|billing|credits'}.items():
   if re.search(pat,alltext):deps.append(dep)
  out['dependency_hints']=deps
  if env:out['risks'].append('environment_variables_or_host_placeholders_used')
  if 'claude_cli' in deps:out['risks'].append('mentions_claude_cli_requires_semantic_review')
  if hookdata:out['risks'].append('hooks_execute_or_prompt_at_runtime_not_tested')
  if mcpdata:out['risks'].append('mcp_connectivity_authentication_and_tool_execution_not_tested')
  if lspdata:out['risks'].append('language_server_installation_and_protocol_not_tested')
  features=[]
  for feat,pat in {'CLAUDE_PLUGIN_ROOT':r'CLAUDE_PLUGIN_ROOT','CLAUDE_PLUGIN_DATA':r'CLAUDE_PLUGIN_DATA','CLAUDE_PROJECT_DIR':r'CLAUDE_PROJECT_DIR','CLAUDE_ENV_FILE':r'CLAUDE_ENV_FILE','dynamic_context':r'(?m)^!`','arguments':r'\$ARGUMENTS|\$[0-9]','context_fork':r'(?m)^context:\s*fork','skill_hooks':r'(?m)^hooks:','allowed_tools':r'(?m)^(?:allowed-tools|tools):','user_invocable':r'(?m)^user-invocable:','disable_model_invocation':r'(?m)^disable-model-invocation:','mcp_tool_names':r'\bmcp__[A-Za-z0-9_-]+__','agent_frontmatter':r'(?m)^(?:model|permissionMode|memory|isolation|background):','shell_script':r'#!/(?:usr/bin/env\s+)?(?:bin/)?(?:ba)?sh','user_config':r'userConfig','dependencies':r'"dependencies"','channels':r'channel','monitors':r'monitor'}.items():
   if re.search(pat,alltext):features.append(feat)
  out['feature_hints']=features
  out['hook_details']=hookdata;out['mcp_details']=mcpdata;out['lsp_details']=lspdata
  def walkdict(x):
   if isinstance(x,dict):
    yield x
    for v in x.values():yield from walkdict(v)
   elif isinstance(x,list):
    for v in x:yield from walkdict(v)
  out['hook_tool_matchers']=sorted(set(str(v['matcher']) for h in hookdata for v in walkdict(h) if 'matcher' in v))
  out['hook_event_names']=sorted(set(k for h in hookdata if isinstance(h,dict) for k in h.get('hooks',h) if isinstance(h.get('hooks',h),dict)))
  out['hook_handler_types']=sorted(set(str(v['type']) for h in hookdata for v in walkdict(h) if 'type' in v))
  hooktext='\n'.join(v for p,v in files.items() if p.startswith('hooks/') or '/hooks/' in p)
  out['hook_script_uses_tool_name']=bool(re.search(r'tool_name',hooktext))
  out['hook_script_capitalized_tool_names']=sorted(set(re.findall(r'\b(?:Bash|Edit|Write|Read|Glob|Grep)\b',hooktext)))
  out['manifest']=manifest
  evpaths=set([manifest_path]+[p for values in comp.values() for p in values if not p.startswith('inline:')])
  # Public snapshots retain actual JSON configuration and skill/command frontmatter, no executable invocation.
  evidence={'repo':repo,'commit':resolved,'path':prefix or '.','archive_sha256':ar['sha256'],'all_plugin_files':paths,'config':{p:files[p] for p in evpaths if p in files and p.endswith('.json')},'markdown_frontmatter':{p:files[p].split('---',2)[1][:16000] if files[p].startswith('---') and len(files[p].split('---',2))>=3 else files[p][:1200] for p in evpaths if p in files and p.endswith('.md')}}
  evidence_name=re.sub(r'[^A-Za-z0-9_.-]','_',market['name']+'--'+str(entry.get('name')))+'--source-evidence.json'
  (SNAP/evidence_name).write_text(json.dumps(evidence,ensure_ascii=False,indent=2),encoding='utf-8')
  out['evidence']=['snapshots/'+evidence_name]
  out['static_status']='static_findings' if out['errors'] else ('static_pass_with_runtime_requirements' if out['risks'] or deps else 'static_pass')
  if not any(comp.values()):out['risks'].append('no_discovered_components');out['static_status']='static_findings'
 except Exception as e:
  out['source_access']='failed_or_limited';out['static_status']='source_unverified';out['errors'].append(type(e).__name__+':'+str(e))
 return out

def main():
 markets=[]
 for p in SNAP.glob('*--marketplace.json'):
  a=p.name.split('--');repo=a[0]+'/'+a[1];commit=a[2];d=json.loads(p.read_text('utf-8-sig'));markets.append({'repo':repo,'commit':commit,'name':d['name'],'entry_count':len(d['plugins']),'snapshot':'snapshots/'+p.name,'plugins':d['plugins']})
 todo=[(e,m) for m in markets if m['repo']!='anthropics/claude-plugins-community' for e in m['plugins']]
 rows=[];started=time.time()
 futures={pool.submit(audit,e,m):(e,m) for e,m in todo}
 for i,f in enumerate(cf.as_completed(futures),1):
  r=f.result();rows.append(r)
  if i%10==0 or r['static_status']=='source_unverified':print(i,'/',len(todo),r['marketplace'],r['name'],r['static_status'],str(r['errors'])[:180],flush=True)
  if i%25==0:(ROOT/'audit-progress.json').write_text(json.dumps({'completed':i,'total':len(todo),'elapsed_seconds':round(time.time()-started),'rows':rows},ensure_ascii=False),encoding='utf-8')
 # Community rows reuse exact fixed source/path evidence where possible. No latest-ref assumption.
 index={(r['source_repo'],r['source_commit'],r['source_path']):r for r in rows if r['source_access']=='archive_accessible'}
 for m in markets:
  if m['repo']!='anthropics/claude-plugins-community':continue
  for e in m['plugins']:
   r=audit(e,m,True);match=index.get((r['source_repo'],r['source_commit'],r['source_path']))
   # Equal source bytes do not establish equal marketplace-supplied components.
   inline_keys=('skills','commands','agents','hooks','mcpServers','lspServers','settings','strict','userConfig','dependencies','outputStyles','workflows','experimental')
   if match and any(e.get(k)!=match['marketplace_entry'].get(k) for k in inline_keys):match=None
   if match:
    keep={k:r[k] for k in ('marketplace','marketplace_repo','marketplace_commit','name','description','source','marketplace_entry')}
    r={**match,**keep,'scan_depth':'reused_exact_source','reused_from':match['marketplace']+':'+match['name']}
   rows.append(r)
 rows.sort(key=lambda x:(x['marketplace_repo'],str(x['name'])))
 summary={'total_entries':len(rows),'source_tree_checked_entries':sum(r['scan_depth']!='catalog_only' and r['source_access']=='archive_accessible' for r in rows),'runtime_tested_entries':0,'by_marketplace':{},'by_status':dict(collections.Counter(r['static_status'] for r in rows)),'unique_declared_sources':len(set((r['source_repo'],r['source_commit'],r['source_path']) for r in rows))}
 for m in markets:
  rr=[r for r in rows if r['marketplace_repo']==m['repo']]
  summary['by_marketplace'][m['repo']]={'entries':len(rr),'commit':m['commit'],'status_counts':dict(collections.Counter(r['static_status'] for r in rr)),'source_tree_checked_entries':sum(r['scan_depth']!='catalog_only' and r['source_access']=='archive_accessible' for r in rr)}
 for r in rows:
  issues=[];e=r['marketplace_entry'];src=e.get('source')
  if not isinstance(e.get('name'),str) or not e['name'].strip():issues.append('name_missing_or_not_string')
  if not isinstance(src,(str,dict)):issues.append('source_missing_or_invalid_type')
  if isinstance(src,dict):
   if not isinstance(src.get('url') or src.get('repo'),str):issues.append('source_url_or_repo_missing')
   if src.get('source')=='git-subdir' and not isinstance(src.get('path'),str):issues.append('git_subdir_path_missing')
   if src.get('sha') and not re.fullmatch('[0-9a-fA-F]{40}',str(src['sha'])):issues.append('source_sha_not_40_hex')
  r['catalog_validation_issues']=issues
 deep=[r for r in rows if r['scan_depth']=='source_tree' and r['source_access']=='archive_accessible']
 summary['catalog_validation_issue_count']=sum(bool(r['catalog_validation_issues']) for r in rows)
 summary['unique_plugin_names']=len({r['name'] for r in rows})
 summary['unique_source_repo_paths']=len({(r['source_repo'],r['source_path']) for r in rows})
 summary['deep_scan']={'attempted_entries':len(todo),'source_tree_checked_entries':len(deep),'unique_source_repo_commit_paths':len({(r['source_repo'],r['source_commit'],r['source_path']) for r in deep}),'component_presence_entries':{k:sum(bool(r['components'].get(k)) for r in deep) for k in ('skills','commands','agents','hooks','mcp','lsp','settings')},'dependency_text_hints':dict(collections.Counter(x for r in deep for x in r['dependency_hints'])),'claude_env_text_mentions_entries':sum(any(x.startswith('CLAUDE_') for x in r['required_env']) for r in deep),'hook_matcher_Bash_Edit_Write_entries':sum(any(re.search(r'\b(?:Bash|Edit|Write)\b',x) for x in r.get('hook_tool_matchers',[])) for r in deep),'hook_script_tool_name_mentions_entries':sum(r.get('hook_script_uses_tool_name',False) for r in deep),'hook_handler_type_presence_entries':dict(collections.Counter(x for r in deep for x in r.get('hook_handler_types',[]))),'hook_event_presence_entries':dict(collections.Counter(x for r in deep for x in r.get('hook_event_names',[])))}
 result={'generated_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'assessment':'STATIC SOURCE AUDIT ONLY; no plugins, commands, hooks, MCPs or external business actions executed; runtime availability and a 90 percent success target remain unverified.','summary':summary,'markets':[{k:v for k,v in m.items() if k!='plugins'} for m in markets],'plugins':rows,'cache_directory':str(CACHE)}
 result['method_notes']=['Source archives are never extracted or executed. No dependency installation, plugin installation, hook execution, MCP connection or external business action.', 'Five-marketplace complete catalog inventory; four-marketplace source scans, plus exact-source evidence reuse in community.', 'Manifest is optional; marketplace inline skills/commands/agents/hooks/MCP/LSP/settings are recognized; unknown/new fields are retained, never rejected just for unfamiliarity.', 'Skill paths add to discovered default skills. Explicit command/agent paths replace default directory discovery per current public documentation.', 'Static pass means only readable source, parseable discovered JSON and component path discovery. This is not full Claude schema/behavior validation.', 'Dependency, CLAUDE environment, channels/monitors and feature text matches may occur in docs/examples/tests; they are investigation hints, not proven execution prerequisites.', 'Component counts count discovered Markdown entries and config locations, not individual tools/servers nor runtime operability.', 'The final compressed archive budget is 25 MiB. Initial exploratory requests used a 40 MiB cap; two 25-40 MiB cached sources are excluded from the final checked result.', 'Unpinned branch references can advance during the audit. Each source is recorded at the actually resolved commit; marketplace snapshot commit is separate.']
 (ROOT/'marketplace-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2),encoding='utf-8')
 fields=['marketplace','marketplace_repo','marketplace_commit','name','source_repo','source_commit','source_path','source_access','scan_depth','static_status','runtime','manifest_status','component_counts','required_env','dependency_hints','feature_hints','hook_tool_matchers','hook_event_names','hook_handler_types','catalog_validation_issues','errors','risks','evidence']
 with (ROOT/'marketplace-audit.csv').open('w',encoding='utf-8-sig',newline='') as f:
  w=csv.DictWriter(f,fieldnames=fields);w.writeheader()
  for r in rows:w.writerow({k:json.dumps(r.get(k),ensure_ascii=False) if isinstance(r.get(k),(dict,list)) else r.get(k) for k in fields})
 used={pathlib.Path(e).name for r in rows for e in r.get('evidence',[])}
 for path in SNAP.glob('*--source-evidence.json'):
  if path.name not in used:path.unlink()
 progress=ROOT/'audit-progress.json'
 if progress.exists():progress.unlink()
 print(json.dumps(summary,ensure_ascii=False,indent=2),flush=True)
if __name__=='__main__':main()
