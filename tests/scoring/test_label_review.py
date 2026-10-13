import importlib.util
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('review',Path(__file__).resolve().parents[2]/'scripts/label-review.py')
review=importlib.util.module_from_spec(spec);spec.loader.exec_module(review)

class ReviewTest(unittest.TestCase):
 def test_unknown_not_negative_and_missing_proof_rejected(self):
  rows=[{'ip':'8.8.8.8','review_status':'unreviewed'}]
  self.assertEqual(review.reviewed(rows),([],1))
  row={'ip':'1.1.1.1','review_status':'approved','labels':{'proxy':False},'label_source':'operator',
       'observed_at':'2026-10-13','reviewed_by':'reviewer','evidence_uri':'private evidence','split_group':'ASN13335',
       'independent':True,'training_overlap':False}
  self.assertEqual(len(review.reviewed([row])[0]),1)
  row['training_overlap']=True
  with self.assertRaises(ValueError):review.reviewed([row])

if __name__=='__main__':unittest.main()
