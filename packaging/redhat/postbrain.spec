Name:           postbrain
Version:        0.0.0
Release:        1%{?dist}
Summary:        Persistent memory and knowledge server for coding agents

License:        Apache-2.0
URL:            https://github.com/simplyblock/postbrain
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang

%description
Postbrain provides server and CLI components for shared memory and knowledge
across coding agents.

%prep
%autosetup

%build
make build

%install
install -D -m 0755 postbrain %{buildroot}%{_bindir}/postbrain
install -D -m 0755 postbrain-cli %{buildroot}%{_bindir}/postbrain-cli
install -D -m 0644 config.example.yaml %{buildroot}%{_sysconfdir}/postbrain/config.yaml

%files
%license LICENSE
%doc README.md
%{_bindir}/postbrain
%{_bindir}/postbrain-cli
%config(noreplace) %{_sysconfdir}/postbrain/config.yaml

%changelog
* Fri Apr 03 2026 Postbrain Team <opensource@simplyblock.io> - 0.0.0-1
- Initial package scaffold
