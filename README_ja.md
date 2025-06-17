<div align="center">
  <a href="https://alist.nn.ci"><img width="100px" alt="logo" src="https://cdn.jsdelivr.net/gh/alist-org/logo@main/logo.svg"/></a>
  <p><em>🗂️Gin と Solidjs による、複数のストレージをサポートするファイルリストプログラム。</em></p>
<div>
  <a href="https://goreportcard.com/report/github.com/alist-org/alist/v3">
    <img src="https://goreportcard.com/badge/github.com/alist-org/alist/v3" alt="latest version" />
  </a>
  <a href="https://github.com/alist-org/alist/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/Xhofe/alist" alt="License" />
  </a>
  <a href="https://github.com/alist-org/alist/actions?query=workflow%3ABuild">
    <img src="https://img.shields.io/github/actions/workflow/status/Xhofe/alist/build.yml?branch=main" alt="Build status" />
  </a>
  <a href="https://github.com/alist-org/alist/releases">
    <img src="https://img.shields.io/github/release/Xhofe/alist" alt="latest version" />
  </a>
  <a title="Crowdin" target="_blank" href="https://crwd.in/alist">
    <img src="https://badges.crowdin.net/alist/localized.svg">
  </a>
</div>
<div>
  <a href="https://github.com/alist-org/alist/discussions">
    <img src="https://img.shields.io/github/discussions/Xhofe/alist?color=%23ED8936" alt="discussions" />
  </a>
  <a href="https://discord.gg/F4ymsH4xv2">
    <img src="https://img.shields.io/discord/1018870125102895134?logo=discord" alt="discussions" />
  </a>
  <a href="https://github.com/alist-org/alist/releases">
    <img src="https://img.shields.io/github/downloads/Xhofe/alist/total?color=%239F7AEA&logo=github" alt="Downloads" />
  </a>
  <a href="https://hub.docker.com/r/xhofe/alist">
    <img src="https://img.shields.io/docker/pulls/xhofe/alist?color=%2348BB78&logo=docker&label=pulls" alt="Downloads" />
  </a>
  <a href="https://alist.nn.ci/guide/sponsor.html">
    <img src="https://img.shields.io/badge/%24-sponsor-F87171.svg" alt="sponsor" />
  </a>
</div>
</div>

---

[English](./README.md) | [中文](./README_cn.md) | 日本語 | [Contributing](./CONTRIBUTING.md) | [CODE_OF_CONDUCT](./CODE_OF_CONDUCT.md)

## 特徴

- [x] マルチストレージ
    - [x] ローカルストレージ
    - [x] [Aliyundrive](https://www.alipan.com/)
    - [x] OneDrive / Sharepoint ([グローバル](https://www.office.com/), [cn](https://portal.partner.microsoftonline.cn),de,us)
    - [x] [189cloud](https://cloud.189.cn) (Personal, Family)
    - [x] [GoogleDrive](https://drive.google.com/)
    - [x] [123pan](https://www.123pan.com/)
    - [x] FTP / SFTP
    - [x] [PikPak](https://www.mypikpak.com/)
    - [x] [S3](https://aws.amazon.com/s3/)
    - [x] [Seafile](https://seafile.com/)
    - [x] [UPYUN Storage Service](https://www.upyun.com/products/file-storage)
    - [x] WebDav(Support OneDrive/SharePoint without API)
    - [x] Teambition([China](https://www.teambition.com/ ),[International](https://us.teambition.com/ ))
    - [x] [Mediatrack](https://www.mediatrack.cn/)
    - [x] [139yun](https://yun.139.com/) (Personal, Family, Group)
    - [x] [YandexDisk](https://disk.yandex.com/)
    - [x] [BaiduNetdisk](http://pan.baidu.com/)
    - [x] [Terabox](https://www.terabox.com/main)
    - [x] [UC](https://drive.uc.cn)
    - [x] [Quark](https://pan.quark.cn)
    - [x] [Thunder](https://pan.xunlei.com)
    - [x] [Lanzou](https://www.lanzou.com/)
    - [x] [ILanzou](https://www.ilanzou.com/)
    - [x] [Aliyundrive share](https://www.alipan.com/)
    - [x] [Google photo](https://photos.google.com/)
    - [x] [Mega.nz](https://mega.nz)
    - [x] [Baidu photo](https://photo.baidu.com/)
    - [x] SMB
    - [x] [115](https://115.com/)
    - [X] Cloudreve
    - [x] [Dropbox](https://www.dropbox.com/)
    - [x] [FeijiPan](https://www.feijipan.com/)
    - [x] [dogecloud](https://www.dogecloud.com/product/oss)
- [x] デプロイが簡単で、すぐに使える
- [x] ファイルプレビュー (PDF, マークダウン, コード, プレーンテキスト, ...)
- [x] ギャラリーモードでの画像プレビュー
- [x] ビデオとオーディオのプレビュー、歌詞と字幕のサポート
- [x] Office ドキュメントのプレビュー (docx, pptx, xlsx, ...)
- [x] `README.md` のプレビューレンダリング
- [x] ファイルのパーマリンクコピーと直接ダウンロード
- [x] ダークモード
- [x] 国際化
- [x] 保護されたルート (パスワード保護と認証)
- [x] WebDav (詳細は https://alist.nn.ci/guide/webdav.html を参照)
- [x] [Docker デプロイ](https://hub.docker.com/r/xhofe/alist)
- [x] Cloudflare ワーカープロキシ
- [x] ファイル/フォルダパッケージのダウンロード
- [x] ウェブアップロード(訪問者にアップロードを許可できる), 削除, mkdir, 名前変更, 移動, コピー
- [x] オフラインダウンロード
- [x] 二つのストレージ間でファイルをコピー
- [x] シングルスレッドのダウンロード/ストリーム向けのマルチスレッド ダウンロード アクセラレーション

## ドキュメント

<https://alist.nn.ci/>

## デモ

<https://al.nn.ci>

## ディスカッション

一般的なご質問は[ディスカッションフォーラム](https://github.com/alist-org/alist/discussions)をご利用ください。**問題はバグレポートと機能リクエストのみです。**

## スポンサー

AList はオープンソースのソフトウェアです。もしあなたがこのプロジェクトを気に入ってくださり、続けて欲しいと思ってくださるなら、ぜひスポンサーになってくださるか、1口でも寄付をしてくださるようご検討ください！すべての愛とサポートに感謝します:
https://alist.nn.ci/guide/sponsor.html

### スペシャルスポンサー

- [VidHub](https://apps.apple.com/app/apple-store/id1659622164?pt=118612019&ct=alist&mt=8) - An elegant cloud video player within the Apple ecosystem. Support for iPhone, iPad, Mac, and Apple TV.
- [亚洲云](https://www.asiayun.com/aff/QQCOOQKZ) - 高防服务器|服务器租用|福州高防|广东电信|香港服务器|美国服务器|海外服务器 - 国内靠谱的企业级云计算服务提供商 (sponsored Chinese API server)
- [找资源](http://zhaoziyuan2.cc/) - 阿里云盘资源搜索引擎

## コントリビューター

これらの素晴らしい人々に感謝します:

[![Contributors](http://contrib.nn.ci/api?repo=alist-org/alist&repo=alist-org/alist-web&repo=alist-org/docs)](https://github.com/alist-org/alist/graphs/contributors)

## ライセンス

`AList` は AGPL-3.0 ライセンスの下でライセンスされたオープンソースソフトウェアです。

## 免責事項
- このプログラムはフリーでオープンソースのプロジェクトです。ネットワークディスク上でファイルを共有するように設計されており、golang のダウンロードや学習に便利です。利用にあたっては関連法規を遵守し、悪用しないようお願いします;
- このプログラムは、公式インターフェースの動作を破壊することなく、公式 sdk/インターフェースを呼び出すことで実装されています;
- このプログラムは、302リダイレクト/トラフィック転送のみを行い、いかなるユーザーデータも傍受、保存、改ざんしません;
- このプログラムを使用する前に、アカウントの禁止、ダウンロード速度の制限など、対応するリスクを理解し、負担する必要があります;
- もし侵害があれば、[メール](mailto:i@nn.ci)で私に連絡してください。

---

> [@Blog](https://nn.ci/) · [@GitHub](https://github.com/alist-org) · [@TelegramGroup](https://t.me/alist_chat) · [@Discord](https://discord.gg/F4ymsH4xv2)
