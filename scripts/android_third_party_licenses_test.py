#!/usr/bin/env python3
"""android/scripts/third-party-licenses.py の --check のテスト(#2857)。

Gradle は回さない(--deps で依存の一覧を渡す)。POM・本文・LICENSE は scripts/testdata/android-licenses/ の小さな偽物を使う。
android/ のスクリプトは読み込むだけで変えない。書き出し先(OUTPUT)は一時フォルダに差し替える。

  python3 -m unittest scripts/android_third_party_licenses_test.py
  python3 scripts/android_third_party_licenses_test.py

testdata の一覧(third_party_licenses.json)を作り直す時:
  UPDATE_GOLDEN=1 python3 scripts/android_third_party_licenses_test.py
"""
import contextlib
import importlib.util
import io
import json
import os
import shutil
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
SCRIPT = os.path.join(REPO, "android", "scripts", "third-party-licenses.py")
DATA = os.path.join(HERE, "testdata", "android-licenses")
GOLDEN = os.path.join(DATA, "third_party_licenses.json")
DEPS = os.path.join(DATA, "deps.txt")
DEPS_ADDED = os.path.join(DATA, "deps-added.txt")


def load_module():
    spec = importlib.util.spec_from_file_location("android_third_party_licenses", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.dont_write_bytecode = True  # android/scripts/ に __pycache__ を作らない
    spec.loader.exec_module(module)
    return module


class AndroidLicensesCheckTest(unittest.TestCase):
    def setUp(self):
        self.lic = load_module()
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)
        self.output = os.path.join(self.tmp, "third_party_licenses.json")
        # 同梱物(whisper.cpp など)とアプリ名は本物の版の更新でテストが揺れないよう固定する
        patches = {
            "GRADLE_CACHE": os.path.join(DATA, "gradle-cache"),
            "TEXTS_DIR": os.path.join(DATA, "license-texts"),
            "APP_LICENSE": os.path.join(DATA, "LICENSE"),
            "REPO_DIR": self.tmp,
            "OUTPUT": self.output,
            "VENDORED": [],
            "APP_ENTRY": {"id": "test-app", "name": "Test App", "url": "https://example.com/app"},
        }
        for name, value in patches.items():
            patcher = mock.patch.object(self.lic, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)

    def run_main(self, *args):
        """(終了コード, stdout, stderr)。SystemExit が無ければ 0。"""
        out, err = io.StringIO(), io.StringIO()
        code = 0
        with mock.patch.object(sys, "argv", ["third-party-licenses.py", *args]), \
                contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            try:
                self.lic.main()
            except SystemExit as e:
                code = e.code if isinstance(e.code, int) else 1
        return code, out.getvalue(), err.getvalue()

    def put_golden(self):
        shutil.copyfile(GOLDEN, self.output)

    def test_golden_is_current(self):
        """testdata の一覧が deps.txt から作った結果と同じ(親 POM の <licenses> も辿る)。"""
        text = self.lic.dump(self.lic.build(self.lic.dependency_report(DEPS)))
        if os.environ.get("UPDATE_GOLDEN"):
            with open(GOLDEN, "w", encoding="utf-8") as f:
                f.write(text)
        with open(GOLDEN, encoding="utf-8") as f:
            self.assertEqual(f.read(), text)
        entries = {e["id"]: e for e in json.loads(text)["entries"]}
        self.assertEqual(entries["com-example"]["artifacts"], ["alpha:1.0.0", "beta:2.0.0"])
        self.assertEqual(entries["com-example"]["license"], "Apache-2.0")
        self.assertIsNone(entries["com-example"]["version"])  # 版が揃っていない
        self.assertNotIn("org-added", entries)

    def test_check_same(self):
        self.put_golden()
        code, out, err = self.run_main("--check", "--deps", DEPS)
        self.assertEqual((code, out), (0, "ok\n"), err)

    def test_check_stale(self):
        with open(GOLDEN, encoding="utf-8") as f:
            data = json.load(f)
        self.assertEqual(data["entries"][1]["id"], "com-example")
        data["entries"][1]["artifacts"] = ["alpha:0.9.0", "beta:2.0.0"]  # 前の版のままの一覧
        with open(self.output, "w", encoding="utf-8") as f:
            f.write(self.lic.dump(data))
        code, out, err = self.run_main("--check", "--deps", DEPS)
        self.assertEqual(code, 1)
        self.assertEqual(out, "")
        self.assertIn("third_party_licenses.json が古いです", err)
        self.assertIn("python3 android/scripts/third-party-licenses.py で作り直してください", err)

    def test_check_missing(self):
        code, _, err = self.run_main("--check", "--deps", DEPS)
        self.assertEqual(code, 1)
        self.assertIn("が古いです", err)

    def test_check_dependency_added(self):
        """依存が増えたのに一覧が前のまま → 古い。作り直すと増えた依存が出て、--check が通る。"""
        self.put_golden()
        code, _, err = self.run_main("--check", "--deps", DEPS_ADDED)
        self.assertEqual(code, 1)
        self.assertIn("が古いです", err)
        with open(self.output, encoding="utf-8") as f, open(GOLDEN, encoding="utf-8") as g:
            self.assertEqual(f.read(), g.read(), "--check が一覧を書き換えた")

        code, out, _ = self.run_main("--deps", DEPS_ADDED)
        self.assertEqual(code, 0)
        self.assertEqual(out, "wrote third_party_licenses.json\n")
        with open(self.output, encoding="utf-8") as f:
            entries = {e["id"]: e for e in json.load(f)["entries"]}
        added = entries["org-added"]
        self.assertEqual(added["artifacts"], ["gamma:3.0.0"])
        self.assertEqual(added["version"], "3.0.0")
        self.assertEqual(added["url"], "https://example.org/gamma")
        self.assertEqual(added["license"], "Apache-2.0")
        self.assertEqual(entries["com-example"]["artifacts"], ["alpha:1.0.0", "beta:2.0.0"])

        self.assertEqual(self.run_main("--check", "--deps", DEPS_ADDED)[:2], (0, "ok\n"))


if __name__ == "__main__":
    unittest.main()
