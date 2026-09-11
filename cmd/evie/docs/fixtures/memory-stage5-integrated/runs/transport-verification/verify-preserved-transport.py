#!/usr/bin/env python3
"""Read-only preservation verification after Git/filesystem transport.

Verify retained bytes and archived original metadata. Direct-file checkout mtimes
and modes are observed but are not evidence that original filesystem metadata
survived Git. This program never restores or modifies original metadata.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import tarfile


def read(path):
    return json.loads(path.read_text())


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda: f.read(1 << 20), b''):
            h.update(b)
    return h.hexdigest()


def regular(path):
    if not path.is_file() or path.is_symlink():
        raise ValueError('expected regular retained file: ' + str(path))


def safe(name):
    p = PurePosixPath(name)
    if p.is_absolute() or '..' in p.parts:
        raise ValueError('unsafe retained member path: ' + name)
    return p


def verify_file(path, expected):
    regular(path)
    if path.stat().st_size != expected['bytes'] or digest(path) != expected['sha256']:
        raise ValueError('content differs from retention manifest: ' + str(path))


def verify_original_archive(path, map_path, archive_sha, map_sha):
    if digest(path) != archive_sha or digest(map_path) != map_sha:
        raise ValueError('source/input archive or manifest differs from frozen identity')
    expected = read(map_path)
    seen = set()
    with tarfile.open(path, 'r:gz') as archive:
        for member in archive:
            safe(member.name)
            if not member.isfile() or member.name not in expected or member.name in seen:
                raise ValueError('unexpected original source/input archive member')
            stream = archive.extractfile(member)
            if hashlib.sha256(stream.read()).hexdigest() != expected[member.name]:
                raise ValueError('original source/input member bytes differ: ' + member.name)
            seen.add(member.name)
    if seen != set(expected):
        raise ValueError('missing original source/input archive member')
    return len(seen)


def verify(directory, repository=None):
    directory = directory.resolve()
    manifest = read(directory / 'preservation-manifest.json')
    plan = read(directory / 'preservation-plan.json')
    files = {str(p.relative_to(directory)) for p in directory.rglob('*') if p.is_file() or p.is_symlink()}
    if files != set(manifest['files']) | {'preservation-manifest.json'}:
        raise ValueError('retained directory has unexpected or missing files')
    for name, expected in manifest['files'].items():
        safe(name)
        verify_file(directory / name, expected)
    if manifest.get('freeze_sha256') != plan.get('freeze_sha256'):
        raise ValueError('plan and copy manifest freeze identities differ')
    metadata_differences = []
    for name, expected in plan['files'].items():
        safe(name)
        p = directory / name
        verify_file(p, expected)
        observed = {'mtime_ns': p.stat().st_mtime_ns, 'mode': p.stat().st_mode & 0o777}
        if any(observed[k] != expected[k] for k in observed):
            metadata_differences.append({'path': name, 'original_recorded': {k: expected[k] for k in observed}, 'checkout_observed': observed})
    archive_members = 0
    for name, expected_archive in plan['archives'].items():
        verify_file(directory / name, expected_archive)
        seen = set()
        with tarfile.open(directory / name, 'r:gz') as archive:
            for member in archive:
                safe(member.name)
                if not member.isfile() or member.name not in expected_archive['members'] or member.name in seen:
                    raise ValueError('unexpected preservation archive member')
                expected = expected_archive['members'][member.name]
                stream = archive.extractfile(member)
                if member.size != expected['bytes'] or hashlib.sha256(stream.read()).hexdigest() != expected['sha256']:
                    raise ValueError('preservation member content differs: ' + member.name)
                mtime = str(expected['mtime_ns'] // 1_000_000_000) + '.' + str(expected['mtime_ns'] % 1_000_000_000).zfill(9)
                if member.pax_headers.get('mtime') != mtime or member.mode != expected['mode']:
                    raise ValueError('archived original metadata differs: ' + member.name)
                seen.add(member.name)
            if seen != set(expected_archive['members']):
                raise ValueError('preservation archive lacks an original member')
        archive_members += len(seen)
    references = plan.get('repository_references', {})
    if references and repository is None:
        raise ValueError('--repository is required to verify referenced repository records')
    for name, expected in references.items():
        safe(name)
        verify_file(repository.resolve() / name, expected)
    source_count = input_count = None
    frozen = directory / 'frozen'
    if (frozen / 'freeze.json').is_file():
        if digest(frozen / 'freeze.json') != plan['freeze_sha256']:
            raise ValueError('preserved freeze identity differs')
        f = read(frozen / 'freeze.json')
        source_count = verify_original_archive(frozen / 'compiled-source.tar.gz', frozen / 'compiled-source-manifest.json', f['compiled_source_archive_sha256'], f['compiled_source_manifest_sha256'])
        input_count = verify_original_archive(frozen / 'prepared-inputs.tar.gz', frozen / 'input-manifest.json', f['prepared_inputs_archive_sha256'], f['input_manifest_sha256'])
    return {'schema_version': 1, 'directory': str(directory), 'content_integrity_verified': True,
        'freeze_sha256': plan.get('freeze_sha256'), 'preservation_manifest_sha256': digest(directory / 'preservation-manifest.json'),
        'retained_files': len(files), 'direct_original_files': len(plan['files']), 'preservation_archive_members': archive_members,
        'source_archive_members': source_count, 'input_archive_members': input_count,
        'repository_records_verified': len(references), 'archived_original_times_and_modes_verified': True,
        'direct_file_original_times_and_modes_claimed_preserved_by_git': False,
        'direct_file_checkout_metadata_difference_count': len(metadata_differences), 'direct_file_checkout_metadata_differences': metadata_differences,
        'limitations': ['Hash integrity is relative to retained manifests and repository identity; no signature or independent authenticity is claimed.',
            'A fresh Git checkout preserves contents, not original direct-file mtimes or complete POSIX modes. Recorded originals remain in the plan and archive metadata.',
            'No executable was rebuilt, model invoked, source tree extracted, or evaluation result reclassified. This is transport verification, not a new measurement or release gate.']}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--directory', type=Path, required=True)
    p.add_argument('--repository', type=Path)
    p.add_argument('--output', type=Path)
    a = p.parse_args()
    result = verify(a.directory, a.repository)
    if a.output:
        target = a.output.resolve()
        if target == a.directory.resolve() or target.is_relative_to(a.directory.resolve()):
            raise ValueError('write the supplemental result outside immutable retained evidence')
        with target.open('x') as out:
            json.dump(result, out, indent=2, sort_keys=True)
            out.write('\n')
    print(json.dumps({k: v for k, v in result.items() if k != 'direct_file_checkout_metadata_differences'}, sort_keys=True))


if __name__ == '__main__':
    main()
