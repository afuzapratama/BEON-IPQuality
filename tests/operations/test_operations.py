import datetime as dt
import importlib.util
import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('operations',Path(__file__).parents[2]/'scripts/operations.py')
ops=importlib.util.module_from_spec(spec);spec.loader.exec_module(ops)

class OperationsTests(unittest.TestCase):
 def test_retention_preserves_recovery_points_and_requires_offsite(self):
  with tempfile.TemporaryDirectory() as tmp,patch.dict(os.environ,{'BACKUP_DIR':tmp},clear=False):
   paths=[]
   for day in range(1,11):
    p=Path(tmp)/f'202601{day:02d}T000000Z-1';p.mkdir();(p/'COMPLETE').touch();(p/'VERIFIED').touch();paths.append(p)
   unrelated=Path(tmp)/'unrelated-data';unrelated.mkdir()
   (Path(tmp)/'20250101T000000Z-1').symlink_to(unrelated,target_is_directory=True)
   latest=paths[-1]
   self.assertEqual(ops.local_prune(latest,dt.datetime(2026,10,12,tzinfo=ops.UTC)),[])
   (latest/'OFFSITE_VERIFIED').touch()
   removed=ops.local_prune(latest,dt.datetime(2026,10,12,tzinfo=ops.UTC))
   self.assertEqual(len(removed),3)
   self.assertTrue(latest.exists());self.assertTrue(unrelated.exists())
   self.assertEqual(len(ops.complete_bundles()),7)
 def test_remote_checksum_mismatch_is_rejected(self):
  body=io.BytesIO(b'corrupted')
  class Client:
   def get_object(self,**kwargs):return {'Body':body}
  with self.assertRaises(RuntimeError):ops.verify_remote(Client(),'bucket','key','0'*64)
  self.assertTrue(body.closed)
 def test_telegram_error_does_not_expose_token(self):
  with patch.dict(os.environ,{'TELEGRAM_BOT_TOKEN':'123:SECRET_TOKEN','TELEGRAM_CHAT_ID':'1'}),patch.object(ops.urllib.request,'urlopen',side_effect=RuntimeError('https://api.telegram.org/bot123:SECRET_TOKEN')):
   with self.assertRaises(RuntimeError) as caught:ops.notify('test')
   self.assertNotIn('SECRET_TOKEN',str(caught.exception))
 def test_malformed_backup_dates_are_not_candidates(self):
  self.assertIsNone(ops.stamp('20261301T000000Z-1'))
  self.assertIsNone(ops.stamp('../20260101T000000Z-1'))

if __name__=='__main__':unittest.main()
