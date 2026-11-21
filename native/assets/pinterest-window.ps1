$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
$handle = [IntPtr]::new([int64]$env:CANVEPP_SITE_HWND)
$root = [Windows.Automation.AutomationElement]::FromHandle($handle)
$documents = New-Object Windows.Automation.PropertyCondition([Windows.Automation.AutomationElement]::ControlTypeProperty, [Windows.Automation.ControlType]::Document)
$waitingForLogin = $env:CANVEPP_SITE_LOGIN -eq 'true'
function Read-OfficialPage {
    $doc = $root.FindFirst([Windows.Automation.TreeScope]::Descendants, $documents)
    if (!$doc) { return }
    $value = $null
    if (!$doc.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern, [ref]$value)) { return }
    $uri = $null
    if ([uri]::TryCreate($value.Current.Value, [UriKind]::Absolute, [ref]$uri) -and $uri.Scheme -eq 'https' -and $uri.Host -in @('www.pinterest.com','pinterest.com')) {
        return [pscustomobject]@{ Document = $doc; Uri = $uri }
    }
}

function Get-ProfileName($doc, $page) {
    $links = New-Object Windows.Automation.PropertyCondition([Windows.Automation.AutomationElement]::ControlTypeProperty, [Windows.Automation.ControlType]::Hyperlink)
    foreach ($link in $doc.FindAll([Windows.Automation.TreeScope]::Descendants, $links)) {
        if ($link.Current.Name -notin @('Your profile','Profile','你的个人资料','您的个人资料','个人资料')) { continue }
        $href = $null
        if (!$link.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern, [ref]$href)) { continue }
        $uri = $null
        if ([uri]::TryCreate($href.Current.Value, [UriKind]::Absolute, [ref]$uri) -and $uri.Scheme -eq 'https' -and $uri.Host -eq $page.Host -and $uri.AbsolutePath -match '^/([^/]+)/?$') { return $Matches[1] }
    }
}

function Test-OwnSaved($path, $profile) {
    return ($profile -and $path -match '^/([^/]+)/(_saved|_pins|pins)/?$' -and $Matches[1] -in @($profile,'me'))
}

# Check the current official document on demand. Usernames and URLs stay here.
if ($env:CANVEPP_SITE_MODE -eq 'probe') {
    $current = Read-OfficialPage
    if (!$current) { return }
    $path = $current.Uri.AbsolutePath
    if ($waitingForLogin -and $path -match '^/login(/|$)') { [Console]::WriteLine('keep'); return }
    $profile = Get-ProfileName $current.Document $current.Uri
    if (Test-OwnSaved $path $profile) { [Console]::WriteLine('keep'); return }
    if ($profile -and $env:CANVEPP_FROM_SAVED -eq 'true' -and $path -match '^/pin/[0-9]+/?$') {
        [Console]::WriteLine('back-to-saved')
        for ($i=0; $i -lt 10; $i++) {
            Start-Sleep -Milliseconds 90
            $current = Read-OfficialPage
            if ($current -and (Test-OwnSaved $current.Uri.AbsolutePath $profile)) { [Console]::WriteLine('keep'); return }
        }
    }
    return
}
# Compile the scroll bridge after sending viewport insets so mounting the
# existing browser and restoring login do not wait for the UIA worker.
$scrollStarted = $false
$previous = ''
$returnStarted = $null
$waitMs = 0
while ($true) {
    if ($waitMs -gt 0) { Start-Sleep -Milliseconds $waitMs }
    $waitMs = 300
    $current = Read-OfficialPage
    if (!$current) { continue }
    $doc = $current.Document
    $page = $current.Uri
    $frame = $root.Current.BoundingRectangle
    $web = $doc.Current.BoundingRectangle
    if ($web.Width -lt 128 -or $web.Height -lt 128) { continue }
    $insets = 'insets {0} {1} {2} {3}' -f [int]($web.Left-$frame.Left), [int]($web.Top-$frame.Top), [int]($frame.Right-$web.Right), [int]($frame.Bottom-$web.Bottom)
    if ($insets -ne $previous) { [Console]::WriteLine($insets); $previous = $insets }
    if (!$scrollStarted) {
        Add-Type -ReferencedAssemblies UIAutomationClient,UIAutomationTypes,WindowsBase -TypeDefinition @'
__CANVEPP_SCROLL_BRIDGE__
'@
        [CanveppScroll]::Start($handle)
        $scrollStarted = $true
    }
    if ($page.AbsolutePath -match '^/login(/|$)') { $waitingForLogin = $true; $waitMs = 650 }
    if ($waitingForLogin -and $page.AbsolutePath -eq '/') {
        # Use the site's own Saved link when the signed-in home exposes it.
        # Keep the existing window and the authenticated renderer alive.
        if (!$returnStarted) {
            $returnStarted = [Diagnostics.Stopwatch]::StartNew()
            $profile = Get-ProfileName $doc $page
            $invoked = $false
            $links = New-Object Windows.Automation.PropertyCondition([Windows.Automation.AutomationElement]::ControlTypeProperty, [Windows.Automation.ControlType]::Hyperlink)
            foreach ($link in $doc.FindAll([Windows.Automation.TreeScope]::Descendants, $links)) {
                $href = $null
                if (!$link.TryGetCurrentPattern([Windows.Automation.ValuePattern]::Pattern, [ref]$href)) { continue }
                $uri = $null
                if (![uri]::TryCreate($href.Current.Value, [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -ne 'https' -or $uri.Host -ne $page.Host -or !(Test-OwnSaved $uri.AbsolutePath $profile)) { continue }
                $invoke = $null
                if ($link.TryGetCurrentPattern([Windows.Automation.InvokePattern]::Pattern, [ref]$invoke)) {
                    try { $invoke.Invoke(); $invoked = $true; break } catch { }
                }
            }
            if (!$invoked) { [Console]::WriteLine('saved-after-login'); break }
        } elseif ($returnStarted.ElapsedMilliseconds -gt 1500) { [Console]::WriteLine('saved-after-login'); break }
    }
    if ($page.AbsolutePath -match '^/[^/]+/(_saved|pins|_pins)/?$') { [Console]::WriteLine('saved-page'); break }
}

if ($scrollStarted) { [CanveppScroll]::Wait() }
