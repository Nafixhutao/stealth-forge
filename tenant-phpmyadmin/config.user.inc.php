<?php
/**
 * Shared phpMyAdmin for free-tier MySQL databases.
 *
 * The server is fixed to the tenant cluster, root login is disabled, and only
 * generated u_* users may authenticate. No credentials are stored here: the
 * operator signs in with the database user and password from the Console.
 */
$cfg['blowfish_secret'] = getenv('PHPMYADMIN_BLOWFISH_SECRET') ?: '';

$i = 0;
$i++;
$cfg['Servers'][$i]['host'] = 'tenant-mysql';
$cfg['Servers'][$i]['port'] = '3306';
$cfg['Servers'][$i]['auth_type'] = 'cookie';
$cfg['Servers'][$i]['AllowNoPassword'] = false;
$cfg['Servers'][$i]['AllowRoot'] = false;

$cfg['AllowArbitraryServer'] = false;
$cfg['CookiePath'] = '/';
$cfg['LoginCookieValidity'] = 3600;
$cfg['ExecTimeLimit'] = 120;
$cfg['MaxRows'] = 200;
$cfg['UploadDir'] = '';
$cfg['SaveDir'] = '';
