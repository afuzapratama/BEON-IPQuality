import importlib.util
import json
from datetime import datetime,timedelta,timezone
from pathlib import Path
import tempfile
import unittest
import subprocess
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('sync',Path(__file__).resolve().parents[2]/'scripts/avastel-sync.py')
sync=importlib.util.module_from_spec(spec);spec.loader.exec_module(sync)

class SyncTest(unittest.TestCase):
 def test_expired_file_never_looks_like_fresh_download(self):
  with tempfile.TemporaryDirectory() as temp:
   root=Path(temp);snapshot=root/'snapshot';snapshot.mkdir();(root/'current').symlink_to('snapshot')
   (snapshot/'proxy-ips.txt').write_text('8.8.8.8\n')
   for age,valid in [(0,True),(8,False)]:
    (snapshot/'manifest.json').write_text(json.dumps({'fetched_at':(datetime.now(timezone.utc)-timedelta(hours=age)).isoformat()}))
    with patch.object(sync,'DATA',root):
     if valid:self.assertEqual(sync.source_snapshot(),snapshot/'proxy-ips.txt')
     else:
      with self.assertRaises(ValueError):sync.source_snapshot()

 def test_prepare_imports_before_bootstrap_without_api_dependency(self):
  with tempfile.TemporaryDirectory() as temp:
   root=Path(temp)/'account'
   def download(command,**kwargs):
    stage=Path(command[command.index('--output')+1]);stage.mkdir()
    (stage/'proxy-ips.txt').write_text('8.8.8.8\n')
    (stage/'manifest.json').write_text('{}')
    return subprocess.CompletedProcess(command,0)
   with patch.object(sync,'DATA',root),patch.object(sync.subprocess,'run',side_effect=download),patch.object(sync,'compose') as compose:
    sync.refresh(prepare_only=True)
    self.assertEqual(compose.call_count,1)
    self.assertIn('ingestor',compose.call_args.args)
    self.assertTrue((root/'current/proxy-ips.txt').exists())

if __name__=='__main__':unittest.main()
