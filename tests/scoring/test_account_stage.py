import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

spec=importlib.util.spec_from_file_location('account',Path(__file__).resolve().parents[2]/'scripts/stage-avastel-account.py')
account=importlib.util.module_from_spec(spec);spec.loader.exec_module(account)


class AccountStageTest(unittest.TestCase):
    def test_csv_dedup(self):
        with tempfile.TemporaryDirectory() as temp:
            directory=Path(temp);archive=directory/'input.zip'
            with zipfile.ZipFile(archive,'w') as z:
                z.writestr('data.csv','ip,isProxy\n8.8.8.8,True\n8.8.8.8,True\n1.1.1.1,False\n')
            result=account.normalize(archive,directory)
            self.assertEqual(result['unique_ips'],1)
            self.assertEqual(result['duplicates'],1)
            self.assertEqual(result['excluded_nonproxy_rows'],1)
            self.assertEqual((directory/'proxy-ips.txt').read_text(),'8.8.8.8\n')

    def test_reject_invalid_and_multi_member_archive(self):
        for content,extra in [('ip,isProxy\n127.0.0.1,True\n',False),('ip,isProxy\n8.8.8.8,Maybe\n',False),('ip\n8.8.8.8\n',True),('ip\n8.8.8.8\n',False),('ip,isProxy\n8.8.8.8,False\n',False)]:
            with tempfile.TemporaryDirectory() as temp:
                directory=Path(temp);archive=directory/'input.zip'
                with zipfile.ZipFile(archive,'w') as z:
                    z.writestr('data.csv',content)
                    if extra:z.writestr('extra.csv',content)
                with self.assertRaises(ValueError):account.normalize(archive,directory)

    def test_token_is_read_as_data(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'key.txt';path.write_text('API_KEY="test-token"')
            self.assertEqual(account.read_key(path),'test-token')
            path.write_text('https://example.org/?api_key=test')
            with self.assertRaises(ValueError):account.read_key(path)


if __name__=='__main__':unittest.main()
