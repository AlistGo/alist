#!/usr/bin/env python3
"""Exercise build.sh failure propagation without network or cross compilers."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
MOCK = '''#!/usr/bin/env python3
import hashlib, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open('calls', 'a') as f: f.write(name + ' ' + ' '.join(args) + '\\n')
if os.environ.get('FAIL') == name: sys.exit(42)
if name == 'git':
    if '--verify' in args: sys.exit(1)
    print('v3.65.0' if 'describe' in args else 'abcdef')
elif name == 'wget': print('{"tag_name": "v3.65.0"}')
elif name == 'curl':
    out = pathlib.Path(args[args.index('-o') + 1])
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(('base.txz ' + hashlib.sha256(b'corrupt' if os.environ.get('FAIL') == 'checksum' else b'archive').hexdigest()) if out.name == 'MANIFEST' else 'archive')
elif name == 'tar':
    if '-zxvf' in args:
        pathlib.Path('public').mkdir(exist_ok=True)
        if 'web-dist-dev.tar.gz' in args: pathlib.Path('web-dist-dev/dist').mkdir(parents=True)
        else: pathlib.Path('dist').mkdir()
    elif '-czvf' in args: pathlib.Path(args[args.index('-czvf')+1]).write_text('packed')
elif name == 'go' and 'build' in args:
    out = pathlib.Path(args[args.index('-o')+1]); out.parent.mkdir(parents=True, exist_ok=True); out.write_text('binary')
elif name == 'sudo':
    os.execvp(args[0], args)
'''

class ReleaseBuildTests(unittest.TestCase):
    def run_build(self, mode, target, fail='', empty=False):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        cwd = Path(tmp.name)
        shutil.copy(ROOT / 'build.sh', cwd)
        (cwd / '.github/scripts').mkdir(parents=True)
        shutil.copy(ROOT / '.github/scripts/build-freebsd.sh', cwd / '.github/scripts')
        (cwd / 'mock-bin').mkdir()
        for name in ['git', 'wget', 'curl', 'tar', 'go', 'sudo']:
            p = cwd / 'mock-bin' / name
            p.write_text(MOCK)
            p.chmod(0o755)
        if mode == 'zip':
            (cwd / 'build').mkdir()
            if not empty: (cwd / 'build/alist-linux-amd64').write_text('binary')
        env = dict(os.environ, PATH=str(cwd / 'mock-bin') + ':' + os.environ['PATH'], FAIL=fail)
        result = subprocess.run(['bash', 'build.sh', mode, target], cwd=cwd, env=env, text=True, capture_output=True)
        return cwd, result

    def test_download_and_compile_failures_stop_before_packaging(self):
        for target in ['linux_musl', 'linux_musl_arm', 'freebsd', 'docker', 'docker-multiplatform']:
            for fail in ['curl', 'go']:
                with self.subTest(target=target, fail=fail):
                    cwd, result = self.run_build('release', target, fail)
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertFalse((cwd / 'build/compress').exists())

    def test_release_package_counts_and_freebsd_metadata(self):
        for target, count in [('linux_musl', 8), ('linux_musl_arm', 11), ('freebsd', 3)]:
            with self.subTest(target=target):
                cwd, result = self.run_build('release', target)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(len(list((cwd / 'build/compress').glob('*.tar.gz'))), count)
                md5_files = list((cwd / 'build/compress').glob('md5*.txt'))
                self.assertEqual(len(md5_files), 1)
                entries = [line.split(maxsplit=1)[1].removeprefix('./') for line in md5_files[0].read_text().splitlines()]
                self.assertEqual(set(entries), {p.name for p in (cwd / 'build/compress').glob('*.tar.gz')})
                calls = (cwd / 'calls').read_text()
                self.assertNotIn('musl.cc', calls)
                if target == 'freebsd':
                    self.assertTrue((cwd / 'build/compress/alist-freebsd-i386.tar.gz').exists())
                    self.assertIn('WebVersion=v3.65.0', calls)
                    self.assertIn('-tags=jsoniter', calls)

    def test_zip_rejects_empty_build_and_skips_metadata_network(self):
        for empty in [True, False]:
            cwd, result = self.run_build('zip', 'md5', empty=empty)
            self.assertEqual(result.returncode == 0, not empty, result.stderr)
            calls = (cwd / 'calls').read_text() if (cwd / 'calls').exists() else ''
            self.assertNotIn('wget', calls)
            self.assertNotIn('git', calls)

    def test_freebsd_checksum_mismatch_stops_compilation(self):
        cwd, result = self.run_build('release', 'freebsd', 'checksum')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('go build', (cwd / 'calls').read_text())
        self.assertFalse((cwd / 'build/compress').exists())

    def test_shared_web_download_failures(self):
        for mode, target in [('dev', ''), ('dev', 'web'), ('beta', 'web'), ('release', ''), ('release', 'android')]:
            with self.subTest(mode=mode, target=target):
                _, result = self.run_build(mode, target, 'curl')
                self.assertNotEqual(result.returncode, 0)

    def test_prepare_download_failure_is_not_cached_as_success(self):
        _, result = self.run_build('prepare', 'docker-multiplatform', 'curl')
        self.assertNotEqual(result.returncode, 0)

if __name__ == '__main__':
    unittest.main()
