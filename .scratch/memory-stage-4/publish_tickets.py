import json
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parent
REPO = 'davidadel66/evie'
MANIFEST = json.loads((ROOT / 'manifest.json').read_text())
STATE_PATH = ROOT / 'publication-state.json'
STATE = json.loads(STATE_PATH.read_text()) if STATE_PATH.exists() else {'issues': {}, 'dependencies': {}}

def gh(*args):
    result = subprocess.run(['gh', *args], text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    return result.stdout.strip()

def save():
    STATE_PATH.write_text(json.dumps(STATE, indent=2) + '\n')

parent_before = json.loads(gh('api', f'repos/{REPO}/issues/131'))
existing = json.loads(gh('issue', 'list', '--repo', REPO, '--state', 'all', '--search', '"Memory Stage 4" in:title', '--limit', '100', '--json', 'number,title,url'))
titles = {i['title']: i for i in existing}
published = ROOT / 'published-bodies'
published.mkdir(exist_ok=True)

for ticket in MANIFEST['tickets']:
    key = str(ticket['draft'])
    title = 'Memory Stage 4: ' + ticket['title']
    body = (ROOT / ticket['body']).read_text()
    for blocker in ticket['blocked_by']:
        prior = STATE['issues'][str(blocker)]
        prior_title = MANIFEST['tickets'][blocker - 1]['title']
        body = body.replace(f'- Draft {blocker:02}: {prior_title}', f"- [{prior_title}]({prior['url']})")
    body_path = published / pathlib.Path(ticket['body']).name
    body_path.write_text(body)
    if key in STATE['issues']:
        issue = STATE['issues'][key]
    elif title in titles:
        issue = titles[title]
    else:
        url = gh('issue', 'create', '--repo', REPO, '--title', title, '--body-file', str(body_path), '--label', 'ready-for-agent')
        issue = {'url': url, 'number': int(url.rsplit('/', 1)[1]), 'title': title}
    remote = json.loads(gh('api', f"repos/{REPO}/issues/{issue['number']}"))
    assert remote['title'] == title, f'Title mismatch: {key}'
    assert remote['body'].strip() == body.strip(), f'Body mismatch: {key}'
    assert 'ready-for-agent' in {label['name'] for label in remote['labels']}, f'Label missing: {key}'
    STATE['issues'][key] = {'number': remote['number'], 'id': remote['id'], 'title': title, 'url': remote['html_url']}
    save()
    print(f"Draft {int(key):02} -> #{remote['number']}: {title}", flush=True)

for ticket in MANIFEST['tickets']:
    key = str(ticket['draft'])
    issue = STATE['issues'][key]
    endpoint = f"repos/{REPO}/issues/{issue['number']}/dependencies/blocked_by"
    current = json.loads(gh('api', endpoint))
    current_numbers = {item['number'] for item in current}
    expected = {STATE['issues'][str(d)]['number'] for d in ticket['blocked_by']}
    for blocker in ticket['blocked_by']:
        prior = STATE['issues'][str(blocker)]
        if prior['number'] not in current_numbers:
            gh('api', '--method', 'POST', endpoint, '-F', f"issue_id={prior['id']}")
    verified = json.loads(gh('api', endpoint))
    assert {item['number'] for item in verified} == expected, f'Dependency mismatch: {key}'
    STATE['dependencies'][key] = sorted(expected)
    save()
    print(f"Verified #{issue['number']} blockers: {sorted(expected)}", flush=True)

parent_after = json.loads(gh('api', f'repos/{REPO}/issues/131'))
for field in ('body', 'title', 'state', 'labels'):
    assert parent_after[field] == parent_before[field], f'Parent changed: {field}'
STATE['status'] = 'published-and-verified'
STATE['parent_unchanged'] = True
save()
print('All 20 issues and native dependencies verified; parent unchanged.', flush=True)
