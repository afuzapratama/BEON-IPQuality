import importlib.util
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('arm_stage',Path(__file__).resolve().parents[2]/'scripts/stage-avastel-arm.py')
arm_stage=importlib.util.module_from_spec(spec);spec.loader.exec_module(arm_stage)


class ArmStageTest(unittest.TestCase):
    def test_pressure_guard_and_timeout_distinct(self):
        mib=1024*1024
        self.assertIsNone(arm_stage.abort_reason(600*mib,300))
        self.assertEqual(arm_stage.abort_reason(599*mib,10),'host memory below 600 MiB')
        self.assertEqual(arm_stage.abort_reason(800*mib,301),'worker timeout exceeded 300 seconds')

    def test_account_signal_remains_proxy_context(self):
        feed=arm_stage.source_config('http://isolated/proxy-ips.txt')
        self.assertEqual(feed['threat_type'],'proxy')
        self.assertEqual(feed['weight']*feed['confidence'],32)
        self.assertEqual(feed['schedule'],'0 */6 * * *')


if __name__=='__main__':unittest.main()
