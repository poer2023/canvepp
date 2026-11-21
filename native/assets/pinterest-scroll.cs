// Hosted by the existing window observer. Only the owned Chrome document's
// scroll range crosses the pipe. No page content or profile data is needed.
using System;
using System.Globalization;
using System.Threading;
using System.Windows.Automation;

public static class CanveppScroll {
    static readonly AutoResetEvent wake = new AutoResetEvent(false);
    static readonly ManualResetEvent done = new ManualResetEvent(false);
    static volatile bool visible = true, closed;
    static double requested = -1;

    public static void Start(IntPtr handle) {
        var reader = new Thread(() => {
            try {
                string line;
                while ((line = Console.ReadLine()) != null) {
                    double value;
                    if (line == "hide") visible = false;
                    else if (line == "show") visible = true;
                    else if (Double.TryParse(line, NumberStyles.Float, CultureInfo.InvariantCulture, out value)
                             && value >= 0 && value <= 100) { visible = true; Interlocked.Exchange(ref requested, value); }
                    wake.Set();
                }
            } finally { closed = true; wake.Set(); }
        });
        reader.IsBackground = true;
        reader.Start();
        var worker = new Thread(() => Observe(handle));
        worker.IsBackground = true;
        worker.SetApartmentState(ApartmentState.MTA);
        worker.Start();
    }

    static void Observe(IntPtr handle) {
        ScrollPattern pattern = null;
        var document = new PropertyCondition(AutomationElement.ControlTypeProperty, ControlType.Document);
        string last = "";
        try {
            while (!closed) {
                if (!visible) { pattern = null; wake.WaitOne(); continue; }
                int delay = 100;
                double pos = -1, view = 100;
                try {
                    if (pattern == null) {
                        var root = AutomationElement.FromHandle(handle);
                        var doc = root.FindFirst(TreeScope.Descendants, document);
                        object p;
                        if (doc != null && doc.TryGetCurrentPattern(ScrollPattern.Pattern, out p))
                            pattern = (ScrollPattern)p;
                    }
                    if (pattern != null) {
                        double target = Interlocked.Exchange(ref requested, -1);
                        if (target >= 0) pattern.SetScrollPercent(ScrollPattern.NoScroll, target);
                        pos = pattern.Current.VerticalScrollPercent;
                        view = pattern.Current.VerticalViewSize;
                    }
                    if (pos < 0 || view >= 100) { pattern = null; delay = 400; }
                } catch (System.Runtime.InteropServices.COMException) { pattern = null; }
                  catch (InvalidOperationException) { pattern = null; }
                  catch (ElementNotAvailableException) { pattern = null; }
                string state = String.Format(CultureInfo.InvariantCulture, "scroll {0:F3} {1:F3}", pos, view);
                if (state != last) { Console.WriteLine(state); last = state; }
                wake.WaitOne(delay);
            }
        } finally { done.Set(); }
    }

    public static void Wait() { done.WaitOne(); }
}
