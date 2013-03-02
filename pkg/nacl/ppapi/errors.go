// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

// Below enumeration and comments based on pp_errors.h from NaCl SDK.

/* Copyright (c) 2012 The Chromium Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

const (
	/**
	 * This value is returned by a function on successful synchronous completion
	 * or is passed as a result to a PP_CompletionCallback_Func on successful
	 * asynchronous completion.
	 */
	PP_OK = 0
	/**
	 * This value is returned by a function that accepts a PP_CompletionCallback
	 * and cannot complete synchronously. This code indicates that the given
	 * callback will be asynchronously notified of the final result once it is
	 * available.
	 */
	PP_OK_COMPLETIONPENDING = -1
	/**This value indicates failure for unspecified reasons. */
	PP_ERROR_FAILED = -2
	/**
	 * This value indicates failure due to an asynchronous operation being
	 * interrupted. The most common cause of this error code is destroying a
	 * resource that still has a callback pending. All callbacks are guaranteed
	 * to execute, so any callbacks pending on a destroyed resource will be
	 * issued with PP_ERROR_ABORTED.
	 *
	 * If you get an aborted notification that you aren't expecting, check to
	 * make sure that the resource you're using is still in scope. A common
	 * mistake is to create a resource on the stack, which will destroy the
	 * resource as soon as the function returns.
	 */
	PP_ERROR_ABORTED = -3
	/** This value indicates failure due to an invalid argument. */
	PP_ERROR_BADARGUMENT = -4
	/** This value indicates failure due to an invalid PP_Resource. */
	PP_ERROR_BADRESOURCE = -5
	/** This value indicates failure due to an unavailable PPAPI interface. */
	PP_ERROR_NOINTERFACE = -6
	/** This value indicates failure due to insufficient privileges. */
	PP_ERROR_NOACCESS = -7
	/** This value indicates failure due to insufficient memory. */
	PP_ERROR_NOMEMORY = -8
	/** This value indicates failure due to insufficient storage space. */
	PP_ERROR_NOSPACE = -9
	/** This value indicates failure due to insufficient storage quota. */
	PP_ERROR_NOQUOTA = -10
	/**
	 * This value indicates failure due to an action already being in
	 * progress.
	 */
	PP_ERROR_INPROGRESS = -11
	/** This value indicates failure due to a file that does not exist. */
	/**
	 * The requested command is not supported by the browser.
	 */
	PP_ERROR_NOTSUPPORTED = -12
	/**
	 * Returned if you try to use a null completion callback to "block until
	 * complete" on the main thread. Blocking the main thread is not permitted
	 * to keep the browser responsive (otherwise, you may not be able to handle
	 * input events, and there are reentrancy and deadlock issues).
	 *
	 * The goal is to provide blocking calls from background threads, but PPAPI
	 * calls on background threads are not currently supported. Until this
	 * support is complete, you must either do asynchronous operations on the
	 * main thread, or provide an adaptor for a blocking background thread to
	 * execute the operaitions on the main thread.
	 */
	PP_ERROR_BLOCKS_MAIN_THREAD = -13
	PP_ERROR_FILENOTFOUND       = -20
	/** This value indicates failure due to a file that already exists. */
	PP_ERROR_FILEEXISTS = -21
	/** This value indicates failure due to a file that is too big. */
	PP_ERROR_FILETOOBIG = -22
	/**
	 * This value indicates failure due to a file having been modified
	 * unexpectedly.
	 */
	PP_ERROR_FILECHANGED = -23
	/** This value indicates failure due to a time limit being exceeded. */
	PP_ERROR_TIMEDOUT = -30
	/**
	 * This value indicates that the user cancelled rather than providing
	 * expected input.
	 */
	PP_ERROR_USERCANCEL = -40
	/**
	 * This value indicates failure due to lack of a user gesture such as a
	 * mouse click or key input event. Examples of actions requiring a user
	 * gesture are showing the file chooser dialog and going into fullscreen
	 * mode.
	 */
	PP_ERROR_NO_USER_GESTURE = -41
	/**
	 * This value indicates that the graphics context was lost due to a
	 * power management event.
	 */
	PP_ERROR_CONTEXT_LOST = -50
	/**
	 * Indicates an attempt to make a PPAPI call on a thread without previously
	 * registering a message loop via PPB_MessageLoop.AttachToCurrentThread.
	 * Without this registration step, no PPAPI calls are supported.
	 */
	PP_ERROR_NO_MESSAGE_LOOP = -51
	/**
	 * Indicates that the requested operation is not permitted on the current
	 * thread.
	 */
	PP_ERROR_WRONG_THREAD = -52
)
