import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('evaluation', Path(__file__).resolve().parents[2] / 'scripts/evaluate-scoring.py')
evaluation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evaluation)


class EvaluationTest(unittest.TestCase):
    def fixtures(self):
        labels, predictions = {}, {}
        for i, (actual, predicted) in enumerate([(True, True), (False, True), (False, False), (True, False)]):
            ip = f'8.8.8.{i+1}'
            labels[ip] = {'labels': {'risky': actual}, 'label_source': 'synthetic unit fixture',
                          'observed_at': '2026-10-13', 'independent': True}
            predictions[ip] = {'risky': predicted, 'data_version': 'fixture', 'score': 80 if predicted else 20}
        return labels, predictions

    def test_confusion_and_undefined_metrics(self):
        labels, predictions = self.fixtures()
        result = evaluation.evaluate(labels, predictions)['categories']
        self.assertEqual({k: result['risky'][k] for k in ('tp', 'fp', 'tn', 'fn')}, dict(tp=1, fp=1, tn=1, fn=1))
        for name in ('precision', 'recall', 'false_positive_rate', 'f1'):
            self.assertEqual(result['risky'][name], 0.5)
        self.assertIsNone(result['proxy']['precision'])

    def test_reject_partial_mixed_or_unreviewed_data(self):
        for change in ('missing', 'mixed', 'unreviewed', 'invalid'):
            labels, predictions = self.fixtures()
            ip = next(iter(labels))
            if change == 'missing': predictions.pop(ip)
            if change == 'mixed': predictions[ip]['data_version'] = 'other'
            if change == 'unreviewed': labels[ip]['independent'] = False
            if change == 'invalid': labels[ip]['labels']['risky'] = 'false'
            with self.assertRaises(ValueError): evaluation.evaluate(labels, predictions)

    def test_threshold_preserves_whitelist(self):
        labels, predictions = self.fixtures()
        predictions['8.8.8.2']['whitelisted'] = True
        result = evaluation.evaluate(labels, predictions, 50)['categories']['risky']
        self.assertEqual(result['fp'], 0)
        self.assertEqual(result['tn'], 2)


if __name__ == '__main__': unittest.main()
